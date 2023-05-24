# CI tools for golang

This is a repo containing common magefile based CI tools for golang projects.

To use it, include a makefile in root of your repo directory

```makefile
# This Makefile is meant to be used by people that do not usually work with Go source code.
# If you know what GOPATH is then you probably don't need to bother with make.

GO_PATH=$(shell go env GOPATH)
DEP_PATH=$(GO_PATH)/bin/dep
MAGE=go run ci/mage.go

default:
ifeq ("$(wildcard $(DEP_PATH))", "")
	go get -u github.com/golang/dep/cmd/dep
endif
	${DEP_PATH} ensure
	${MAGE} -l

% :
ifeq ("$(wildcard $(DEP_PATH))", "")
	go get -u github.com/golang/dep/cmd/dep
endif
	${DEP_PATH} ensure
	${MAGE} $(MAKECMDGOALS)

```


Then, create a `magefile.go` file to contain all the mage files you need. To include the common scripts from this library, a following file is suggested:

```golang
// Runs the test suite against the repo
func Test() error {
	return commands.Test("./...")
}

// Checks for copyright headers in files
func CheckCopyright() error {
	return commands.Copyright("./...")
}

// Checks for issues with go imports
func CheckGoImports() error {
	return commands.GoImports("./...")
}

// Reports linting errors in the solution
func CheckGoLint() error {
	return commands.GoLint("./...")
}

// Updates the go report for the repo
func CheckGoReport() error {
	return commands.GoReport("github.com/your-name/your-repo")
}

// Checks that the source is compliant with go vet
func CheckGoVet() error {
	return commands.GoVet("./...")
}

// Checks that the source is compliant with go vet
func Check() error {
	return commands.Check("./...")
}

```

Deployment helpers in `magedeploy.go`:

```go
//go:build mage
// +build mage

package main

import (
	"context"

	"github.com/magefile/mage/mg"
	"github.com/zolia/go-ci/deploy"
	"github.com/zolia/go-ci/notify"
)

const project = "optimus-prime"
const slackURL = "https://hooks.slack.com/services/x/x/x"

var testEnv = deploy.Env{
	Project: project,
	Env:     "test",
	SSHAddr: "deploy@x.x.x.x",
}

var devEnv = deploy.Env{
	Project: project,
	Env:     "dev",
	SSHAddr: "deploy@x.x.x.x",
}

var prodEnv = deploy.Env{
	Project: project,
	Env:     "prod",
	SSHAddr: "deploy@x.x.x.x",
}

// DeployTest deploy to the test env, but fail if remote .env is different
func DeployTest() error {
	return deployEnv(testEnv, true)
}

// DeployTestF deploy to the test env overriding the remote .env
func DeployTestF() error {
	return deployEnv(testEnv, false)
}

// DeployDev deploy to the dev env, but fail if remote .env is different
func DeployDev() error {
	return deployEnv(devEnv, true)
}

// DeployDevF deploy to the dev env overriding the remote .env
func DeployDevF() error {
	return deployEnv(devEnv, false)
}

// DeployProd deploy to the prod env, but fail if remote .env is different
func DeployProd() error {
	return deployEnv(prodEnv, true)
}

// DeployProdF deploy to the prod env overriding the remote .env
func DeployProdF() error {
	return deployEnv(prodEnv, false)
}

func deployEnv(env deploy.Env, validateEnv bool) error {
	mg.Deps(Swag)
	mg.CtxDeps(context.Background(), BuildOctopusLinux)

	cfg := getDeploymentConfig(env)
	if validateEnv {
		err := deploy.ValidateDeploymentEnv(cfg)
		if err != nil {
			return err
		}
	}

	return deploy.Deploy(cfg)
}

func getDeploymentConfig(env deploy.Env) deploy.DeployConfig {
	message := notify.DeploymentMessage{
		SlackURL:   slackURL,
		Env:        env.Env,
		Author:     "CI",
		AuthorLink: "https://ci.optimus-prime.com/machines/" + project,
	}

	cfg := deploy.Config{
		Service: project,
		BinDir:  "build",
		SSHAddr: env.SSHAddr,
		SSHKey:  "~/.ssh/deployment_key",
		Env:     env.Env,
		HomeDir: "op-" + project,
		Success: func() error {
			message.Color = notify.Good
			return notify.SlackDeployment(message)
		},
		Error: func(err error) error {
			message.Color = notify.Bad
			message.Error = err.Error()
			return notify.SlackDeployment(message)
		},
	}

	return cfg
}

```

With this, just run make in the root of your repo and you're set!
