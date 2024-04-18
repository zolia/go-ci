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
	// KeystoreDir certificate key stores
	CopyDst map[string][]string
	// Directory configured for garbage collector logs
	GCLogsDir string
	// Directory configured for application logs
	LogsDir string
	// Source directory where application jar resides
	BinDir string
	// LogFileName is name of the log file to copy to remote
	LogFileName func() string
	// Files to copy to remote
	Files []string
	// Setup systemd service
	SystemD bool
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
	fmt.Printf("construct deploy commands to %s env...\n", cfg.Env)
	commands := deployCommands(cfg)

	fmt.Printf("deploying to %s env...\n", cfg.Env)
	err := deploy(cfg, commands)
	if err != nil {
		fmt.Printf("failed to deploy: %v", err)

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

func setupInitDCommands(cfg Config) []Command {
	initDFile := fmt.Sprintf("%s/%s.jar", cfg.HomeDir, cfg.Service)
	fmt.Printf("setting up init.d service for: %s\n", initDFile)
	initDLink := fmt.Sprintf("/etc/init.d/%s", cfg.Service)

	return []Command{
		{
			Name: "setup init.d service",
			Cmd:  "ssh",
			Args: []string{"sudo", "ln", "-sf", initDFile, initDLink},
		},
		{
			Name: "setup init.d service permissions",
			Cmd:  "ssh",
			Args: []string{"sudo", "chmod", "+x", initDFile},
		},
	}
}

func setupSystemDCommands(cfg Config) []Command {
	initDFile := fmt.Sprintf("%s/%s.jar", cfg.HomeDir, cfg.Service)

	systemDFile := fmt.Sprintf("/etc/systemd/system/%s.service", cfg.Service)
	// Define the content of the service file

	systemDFileContent := `[Unit]
Description={serviceName} service
After=network.target

[Service]
Type=forking
ExecStart=/etc/init.d/{serviceName} start
ExecStop=/etc/init.d/{serviceName} stop
PIDFile=/run/{serviceName}/{serviceName}.pid
Restart=always
LimitNOFILE=65536
RestartSec=5

[Install]
WantedBy=multi-user.target`

	// Replace the placeholders in the service file content
	systemDFileContent = strings.ReplaceAll(systemDFileContent, "{serviceName}", cfg.Service)
	systemDFileContent = "'" + systemDFileContent + "'"
	fmt.Printf("setting up systemd service for: %s\n", systemDFile)

	return []Command{
		{
			Name: "setup init.d service permissions",
			Cmd:  "ssh",
			Args: []string{"sudo", "chmod", "+x", initDFile},
		},
		{
			Name: "create systemd service file",
			Cmd:  "ssh",
			Args: []string{"echo", systemDFileContent, "|", "sudo", "tee", systemDFile},
		},
	}
}

func deploy(cfg Config, commands []Command) error {
	for _, cmd := range commands {
		fmt.Println("\033[34m-", cmd.Name, "\033[0m")

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
	// setup init.d service
	prepareCommands := []Command{
		{
			Name: "create project dir in home",
			Cmd:  "ssh",
			Args: []string{"[ ! -d " + cfg.HomeDir + " ] && mkdir -p", cfg.HomeDir, "&"},
		},
		{
			Name: "create gc logs directory",
			Cmd:  "ssh",
			Args: []string{"[ ! -d ~/" + cfg.GCLogsDir + " ] && mkdir -p", cfg.GCLogsDir, "&"},
		},
		{
			Name: "create logs directory",
			Cmd:  "ssh",
			Args: []string{"[ ! -d ~/" + cfg.LogsDir + " ] && mkdir -p", cfg.LogsDir, "&"},
		},
	}

	if cfg.CopyDst != nil {
		for dir := range cfg.CopyDst {
			prepareCommands = append(prepareCommands, Command{
				Name: "create dir in remote",
				Cmd:  "ssh",
				Args: []string{"[ ! -d " + dir + " ] && mkdir -p", dir, "&"},
			})

			for _, f := range cfg.CopyDst[dir] {
				// copy files to remote
				prepareCommands = append(prepareCommands, Command{
					Name: "copy " + f,
					Cmd:  "scp",
					Args: []string{f, fmt.Sprintf("%s:%s", cfg.SSHAddr, dir)},
				})
			}
		}
	}

	var copyCommands []Command
	targetRemoteHome := fmt.Sprintf("%s:%s", cfg.SSHAddr, cfg.HomeDir)
	for _, file := range cfg.Files {
		copyCommands = append(copyCommands, Command{
			Name: "copy " + file,
			Cmd:  "scp",
			Args: []string{file, targetRemoteHome},
		})
	}

	serviceWithHomeDir := fmt.Sprintf("%s/%s", cfg.HomeDir, cfg.Service)

	uploadCommands := []Command{
		{
			Name:         "stopping existing service",
			Cmd:          "ssh",
			Args:         []string{"sudo", "service", cfg.Service, "stop"},
			IgnoreFailed: true,
		},
		{
			Name:         "backup existing properties file",
			Cmd:          "ssh",
			Args:         []string{"sudo", "mv", serviceWithHomeDir + ".properties", serviceWithHomeDir + ".properties.bak"},
			IgnoreFailed: true,
		},
		{
			Name:         "rename new properties file",
			Cmd:          "ssh",
			Args:         []string{"sudo", "mv", serviceWithHomeDir + ".properties." + cfg.Env, serviceWithHomeDir + ".properties"},
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
	}

	var initdCommands []Command
	initdCommands = setupInitDCommands(cfg)

	var systemdCommands []Command
	if cfg.SystemD {
		systemdCommands = setupSystemDCommands(cfg)
	}

	startCommands := []Command{
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
			Name: "enable service",
			Cmd:  "ssh",
			Args: []string{"sudo", "systemctl", "enable", cfg.Service},
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
	allCommands = append(allCommands, uploadCommands...)
	allCommands = append(allCommands, initdCommands...)
	allCommands = append(allCommands, systemdCommands...)
	allCommands = append(allCommands, startCommands...)
	return allCommands
}
