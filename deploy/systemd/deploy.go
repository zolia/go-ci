/*
 *  Copyright (c) 2023.  Antanas Masevicius
 *
 *  This source code is licensed under the MIT license found in the
 *  LICENSE file in the root directory of this source tree.
 *
 */

package systemd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/magefile/mage/sh"

	"github.com/zolia/go-ci/deploy"
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
	// Service executable name
	ServiceExecutable string
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
	// Directory configured for application logs
	LogsDir string
	// Source directory where application resides
	BinDir string
	// LogFileName is name of the log file to copy to remote
	LogFileName func() string
	// Files to copy to remote
	Files []string
	// Copy files to remote and rename
	FilesToFiles map[string]string
	// Setup systemd service
	SystemD bool
	// Max open files limit
	MaxOpenFiles int
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
	fmt.Printf("construct deployCommands commands to %s env...\n", cfg.Env)
	commands := createDeployCommands(cfg)

	fmt.Printf("deploying to %s env...\n", cfg.Env)
	err := deployCommands(cfg, commands)
	if err != nil {
		fmt.Printf("failed to deployCommands: %v", err)

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

func setupSystemDCommands(cfg Config) []Command {
	systemDFile := fmt.Sprintf("/etc/systemd/system/%s.service", cfg.Service)
	// Define the content of the service file

	if cfg.MaxOpenFiles <= 0 {
		cfg.MaxOpenFiles = 65536 // default value
	}

	systemDFileContent := `[Unit]
Description={service} service
After=network.target

[Service]
Type=simple
WorkingDirectory={homeDir}
ExecStart={homeDir}/{service}
Restart=always
LimitNOFILE={maxOpenFiles}
RestartSec=15

[Install]
WantedBy=multi-user.target`

	// Replace the placeholders in the service file content
	systemDFileContent = strings.ReplaceAll(systemDFileContent, "{homeDir}", cfg.HomeDir)
	systemDFileContent = strings.ReplaceAll(systemDFileContent, "{service}", cfg.Service)
	systemDFileContent = strings.ReplaceAll(systemDFileContent, "{maxOpenFiles}", strconv.Itoa(cfg.MaxOpenFiles))
	systemDFileContent = "'" + systemDFileContent + "'"
	fmt.Printf("setting up systemd service for: %s\n", systemDFile)

	return []Command{
		{
			Name: "create systemd service file",
			Cmd:  "ssh",
			Args: []string{"echo", systemDFileContent, "|", "sudo", "tee", systemDFile},
		},
	}
}

func deployCommands(cfg Config, commands []Command) error {
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

func createDeployCommands(cfg Config) []Command {
	// setup init.d service
	prepareCommands := []Command{
		{
			Name: "create project dir in home",
			Cmd:  "ssh",
			Args: []string{"[ ! -d " + cfg.HomeDir + " ] && mkdir -p", cfg.HomeDir, "&"},
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

	// Copy files to remote and rename
	if cfg.FilesToFiles != nil {
		for file, remoteFile := range cfg.FilesToFiles {
			copyCommands = append(copyCommands, Command{
				Name: "copy " + file,
				Cmd:  "scp",
				Args: []string{file, fmt.Sprintf("%s:%s", cfg.SSHAddr, remoteFile)},
			})
		}
	}
	// serviceWithHomeDir := fmt.Sprintf("%s/%s", cfg.HomeDir, cfg.Service)

	uploadCommands := []Command{
		{
			Name: "copy binary to remote as a tmp file",
			Cmd:  "scp",
			Args: []string{
				fmt.Sprintf("%s/%s", cfg.BinDir, cfg.ServiceExecutable),
				fmt.Sprintf("%s/%s", targetRemoteHome, cfg.Service+".tmp"),
			},
		},
		{
			Name:         "stopping existing service",
			Cmd:          "ssh",
			Args:         []string{"sudo", "service", cfg.Service, "stop"},
			IgnoreFailed: true,
		},
		{
			Name: "backup old service executable",
			Cmd:  "ssh",
			Args: []string{"cp", "-f", fmt.Sprintf("%s/%s", cfg.HomeDir, cfg.Service), fmt.Sprintf("%s/%s.bak", cfg.HomeDir, cfg.Service)},
		},
		{
			Name: "move tmp file to service executable",
			Cmd:  "ssh",
			Args: []string{"mv", fmt.Sprintf("%s/%s.tmp", cfg.HomeDir, cfg.Service), fmt.Sprintf("%s/%s", cfg.HomeDir, cfg.Service)},
		},
	}

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
	allCommands = append(allCommands, systemdCommands...)
	allCommands = append(allCommands, startCommands...)
	return allCommands
}

// ValidateDeploymentEnv validates local .env_<x> against remote .env_<x>
func ValidateDeploymentEnv(cfg Config) error {
	// if remote .env does not exists, skip validation
	_, err := sh.Output("ssh", "-i", cfg.SSHKey, cfg.SSHAddr, "ls", cfg.HomeDir+"/.env")
	if err != nil {
		fmt.Printf("remote .env %s does not exist yet, skipping validation\n", cfg.Env)
		return nil
	}

	envFile := fmt.Sprintf(".env_%s", cfg.Env)
	valid, err := deploy.ValidateEnv(cfg.HomeDir, []string{"-i", cfg.SSHKey, cfg.SSHAddr}, envFile, true)
	if err != nil {
		return fmt.Errorf("failed to diff env files: %w", err)
	}

	if !valid {
		return fmt.Errorf("%s does not match remote .env", envFile)
	}

	return nil
}
