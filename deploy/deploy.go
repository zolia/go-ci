package deploy

import (
	"fmt"
	"time"

	"github.com/magefile/mage/sh"
)

// Command is a command to run during deployment
type Command struct {
	Name string
	Cmd  string
	Args []string
	Func func() error
}

// Env is a deployment environment
type Env struct {
	Project string
	Env     string
	SSHAddr string
}

// Config is a config for deployment
type Config struct {
	// Service binary name
	Service string
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
	Files map[string][]string
	// possible commands to run before/after launch
	// PreLaunch  []Command
	// PostLaunch []Command
	// no need to expose for now
	// SCPArgs []string
	// SSHArgs []string
	// SSHKey is path to ssh key
	Success func() error
	Error   func(err error) error
}

// ValidateDeploymentEnv validates local .env_<x> against remote .env_<x>
func ValidateDeploymentEnv(cfg Config) error {
	envFile := fmt.Sprintf(".env_%s", cfg.Env)
	valid, err := ValidateEnv(cfg.HomeDir, []string{"-i", cfg.SSHKey, cfg.SSHAddr}, envFile, true)
	if err != nil {
		return fmt.Errorf("failed to diff env files: %w", err)
	}

	if !valid {
		return fmt.Errorf("%s does not match remote .env", envFile)
	}

	return nil
}

// Deploy deploys files to remote and runs commands
func Deploy(cfg Config) error {
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
			return fmt.Errorf("unknown command: %s (supported: ssh,scp)", cmd.Cmd)
		}

		if err := sh.RunV(cmd.Cmd, args...); err != nil {
			return fmt.Errorf("failed to run %s: %w", cmd.Name, err)
		}
	}

	fmt.Printf("deployed to %s env!\n", cfg.Env)

	return nil
}

func deployCommands(cfg Config) []Command {
	prepareCommands := []Command{
		{
			Name: "killing existing service",
			Cmd:  "ssh",
			Args: []string{"killall", cfg.Service, "&"},
		},
		{
			Name: "create project dir in home",
			Cmd:  "ssh",
			Args: []string{"[ ! -d ~/" + cfg.HomeDir + " ] && mkdir -p", cfg.HomeDir, "&"},
		},
		{
			Name: "copy binary to remote",
			Cmd:  "scp",
			Args: []string{fmt.Sprintf("%s/%s", cfg.BinDir, cfg.Service), fmt.Sprintf("%s:~/%s", cfg.SSHAddr, cfg.HomeDir)},
		},
		{
			Name: "copy .env file to remote",
			Cmd:  "scp",
			Args: []string{".env_" + cfg.Env, fmt.Sprintf("%s:~/%s/%s", cfg.SSHAddr, cfg.HomeDir, ".env")},
		},
	}

	copyCommands := []Command{}
	for _, files := range cfg.Files {
		for _, file := range files {
			copyCommands = append(copyCommands, Command{
				Name: "copy " + file,
				Cmd:  "scp",
				Args: []string{file, fmt.Sprintf("%s:~/%s", cfg.SSHAddr, cfg.HomeDir)},
			})
		}
	}

	logFile := fmt.Sprintf("%s_%s.log", cfg.Service, time.Now().Format("2006_01_02_15"))
	if cfg.LogFileName != nil {
		logFile = cfg.LogFileName()
	}

	startCommands := []Command{
		{
			Name: "start service",
			Cmd:  "ssh",
			Args: []string{"cd", fmt.Sprintf("~/%s", cfg.HomeDir), "&&", "coproc", "./" + cfg.Service, "&>>", logFile, "&"},
		},
		{
			Name: "check service status",
			Cmd:  "ssh",
			Args: []string{"echo", cfg.Service + " pid:", "`pidof " + cfg.Service + "`"},
		},
	}

	allCommands := append(prepareCommands, copyCommands...)
	allCommands = append(allCommands, startCommands...)
	return allCommands
}
