/*
 *  Copyright (c) 2023.  Antanas Masevicius
 *
 *  This source code is licensed under the MIT license found in the
 *  LICENSE file in the root directory of this source tree.
 *
 */

package java

import (
	"fmt"
	"strings"

	"github.com/magefile/mage/sh"
)

// Command is a command to run during deployment
type Command struct {
	Name         string
	Cmd          string
	Args         []string
	IgnoreFailed bool
	Func         func() error
}

// Env is a deployment environment
type Env struct {
	// Project name used for home dir
	Project string
	// Env is the name of the environment and .env file
	Env     string
	SSHAddr string
}

// Config is a config for deployment
type Config struct {
	// Service name
	Service string
	// Service jar name
	ServiceJAR string
	// Host is user@host string
	SSHAddr string
	// ~/.ssh/<x>
	SSHKey string
	// Env file ".env.<x>"
	Env string
	// HomeDir is tilde + project dir on the remote "~/<x>"
	HomeDir string
	BinDir  string
	// LogFileName is name of the log file to copy to remote
	LogFileName func() string
	// Files to copy to remote
	Files []string
	// Initialise service as init.d service
	InitD bool
	// possible commands to run before/after launch
	Prepare []Command
	// PostLaunch []Command
	// no need to expose for now
	// SCPArgs []string
	// SSHArgs []string
	// SSHKey is path to ssh key
	Success func() error
	Error   func(err error) error
}

// Deploy deploys files to remote and runs commands
func Deploy(cfg Config) error {
	// setup init.d service
	if cfg.InitD {
		err := setupInitD(cfg)
		if err != nil {
			return err
		}
	}

	commands := deployCommands(cfg)

	err := deploy(cfg, commands)
	if err != nil {
		if cfg.Error != nil {
			return cfg.Error(err)
		}

		return err
	}

	if cfg.Success != nil {
		return cfg.Success()
	}

	return nil
}

func setupInitD(cfg Config) error {
	// check if init.d service exists
	if CheckServiceExists(cfg) {
		return nil
	}

	initDFile := fmt.Sprintf("%s/%s.jar", cfg.HomeDir, cfg.Service)
	fmt.Printf("setting up init.d service for: %s\n", initDFile)
	initDLink := fmt.Sprintf("/etc/init.d/%s", cfg.Service)
	err := sh.RunV("ssh", "-i", cfg.SSHKey, cfg.SSHAddr, "sudo", "ln", "-s", initDFile, initDLink)
	if err != nil {
		return fmt.Errorf("failed to setup init.d service: %w", err)
	}
	err = sh.RunV("ssh", "-i", cfg.SSHKey, cfg.SSHAddr, "sudo", "chmod", "+x", "/etc/init.d/"+cfg.Service)
	if err != nil {
		return fmt.Errorf("failed to setup init.d service: %w", err)
	}
	err = sh.RunV("ssh", "-i", cfg.SSHKey, cfg.SSHAddr, "sudo", "update-rc.d", cfg.Service, "defaults")
	if err != nil {
		return fmt.Errorf("failed to setup init.d service: %w", err)
	}
	return nil
}

func CheckServiceExists(cfg Config) bool {
	fmt.Printf("Checking if init.d service exists: %s\n", cfg.Service)
	// The command `test -x /etc/init.d/SERVICE && sudo /etc/init.d/SERVICE status` checks if the service
	// script is executable and if so, tries to get its status.
	checkCmd := fmt.Sprintf("test -x /etc/init.d/%s && sudo /etc/init.d/%s status", cfg.Service, cfg.Service)

	// Run the command on the remote system
	output, err := sh.Output("ssh", "-i", cfg.SSHKey, cfg.SSHAddr, checkCmd)

	// If the command was successful, the service exists
	if err == nil {
		fmt.Printf("init.d service already exists: %s - skipping creation\n", cfg.Service)
		return true
	}

	fmt.Printf("init.d service check returned: %v\n", output)

	// If the command failed with an error message containing "Not running", the service exists
	if strings.Contains(output, "Not running") {
		return true
	}

	return false
}

func deploy(cfg Config, commands []Command) error {
	for _, cmd := range commands {
		fmt.Println("-", cmd.Name)

		// run command Func if provided
		if cmd.Func != nil {
			if err := cmd.Func(); err != nil {
				return fmt.Errorf("failed to run %s: %w", cmd.Name, err)
			}
			continue
		}

		fmt.Printf("running: %s: %+v\n", cmd.Cmd, cmd.Args)
		args := []string{}
		if cmd.Cmd == "ssh" {
			args = append([]string{"-i", cfg.SSHKey, cfg.SSHAddr}, cmd.Args...)
		} else if cmd.Cmd == "scp" {
			args = append([]string{"-i", cfg.SSHKey}, cmd.Args...)
		} else {
			args = append(args, cmd.Args...)
			// uncomment to blacklist other commands
			// return fmt.Errorf("unknown command: %s (supported: ssh,scp)", cmd.Cmd)
		}

		if err := sh.RunV(cmd.Cmd, args...); err != nil {
			if cmd.IgnoreFailed {
				fmt.Printf("ignoring failed command: %s\n", cmd.Name)
				continue
			}
			return fmt.Errorf("failed to run %s: %w", cmd.Name, err)
		}
	}

	fmt.Printf("deployed to %s env!\n", cfg.Env)

	return nil
}

func deployCommands(cfg Config) []Command {
	prepareCommands := []Command{
		{
			Name: "create project dir in home",
			Cmd:  "ssh",
			Args: []string{"[ ! -d " + cfg.HomeDir + " ] && mkdir -p", cfg.HomeDir, "&"},
		},
	}

	copyCommands := []Command{}
	targetRemoteHome := fmt.Sprintf("%s:%s", cfg.SSHAddr, cfg.HomeDir)

	for _, file := range cfg.Files {
		copyCommands = append(copyCommands, Command{
			Name: "copy " + file,
			Cmd:  "scp",
			Args: []string{file, targetRemoteHome},
		})
	}

	startCommands := []Command{
		{
			Name:         "stopping existing service",
			Cmd:          "ssh",
			Args:         []string{"sudo", "service", cfg.Service, "stop"},
			IgnoreFailed: true,
		},
		{
			Name: "copy binary to remote",
			Cmd:  "scp",
			Args: []string{
				fmt.Sprintf("%s/%s", cfg.BinDir, cfg.ServiceJAR),
				fmt.Sprintf("%s/%s.jar", targetRemoteHome, cfg.Service),
			},
		},
		{
			Name: "reload systemctl daemon",
			Cmd:  "ssh",
			Args: []string{"sudo", "systemctl", "daemon-reload"},
		},
		{
			Name: "starting service",
			Cmd:  "ssh",
			Args: []string{"sudo", "service", cfg.Service, "start"},
		},
		{
			Name: "check service status",
			Cmd:  "ssh",
			Args: []string{"sudo", "service", cfg.Service, "status"},
		},
	}

	allCommands := make([]Command, 0)
	if cfg.Prepare != nil {
		allCommands = append(allCommands, cfg.Prepare...)
	}
	allCommands = append(allCommands, prepareCommands...)
	allCommands = append(allCommands, copyCommands...)
	allCommands = append(allCommands, startCommands...)
	return allCommands
}
