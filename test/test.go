/*
 * Copyright (c) 2020.  Monimoto, UAB. All Rights Reserved.
 *
 * This software contains the intellectual property of Monimoto, UAB. Use of this software and the intellectual
 * property contained therein is expressly limited to the terms and conditions of the License Agreement under
 * which it is provided by or on behalf of Monimoto, UAB.
 */

package test

import (
	"fmt"
	"os"
	"strings"

	"github.com/magefile/mage/sh"
	log "github.com/sirupsen/logrus"
)

// Config test runner config
type Config struct {
	Timeout  string
	Parallel bool
}

// DefaultConfig returns the default config
func DefaultConfig() Config {
	return Config{
		Timeout:  "4m",
		Parallel: true,
	}
}

// GoTestExclude run tests excluding certain directories
func GoTestExclude(cfg Config, excludes ...string) error {
	moduleName, err := getModuleName()
	if err != nil {
		return fmt.Errorf("failed to get module name: %w", err)
	}

	packages, err := getPackagesToTest(moduleName, excludes)

	if err != nil {
		return fmt.Errorf("failed to get packages to test %w", err)
	}

	args := []string{"test", "--count=1", "-race", "-cover", "-timeout", cfg.Timeout}

	if os.Getenv("CI") == "true" {
		log.Info("CI env detected")
		args = append(args, "-tags", "ci")
	}

	if cfg.Parallel {
		args = append(args, packages...)

		err = sh.RunV("go", args...)
		if err != nil {
			return fmt.Errorf("failed to run test: %w", err)
		}

		return nil
	}

	for i := 0; i < len(packages); i++ {
		var cmdArgs = args
		cmdArgs = append(cmdArgs, packages[i])
		err = sh.RunV("go", cmdArgs...)
		if err != nil {
			return fmt.Errorf("failed to run test: %w", err)
		}
	}

	return nil
}

// GoTestInclude run go tests for given directories
func GoTestInclude(includes ...string) error {
	moduleName, err := getModuleName()
	if err != nil {
		return fmt.Errorf("failed to get module name: %w", err)
	}

	// prepend module name
	var fullPackageNames []string
	for _, pkg := range includes {
		fullPackageNames = append(fullPackageNames, moduleName+"/"+pkg)
	}

	args := []string{"test", "-tags", "ci", "--count=1", "--short", "-race", "-cover", "-timeout", "4m"}
	args = append(args, fullPackageNames...)

	err = sh.RunV("go", args...)
	if err != nil {
		return fmt.Errorf("failed to run test: %w", err)
	}
	return nil
}

func getModuleName() (string, error) {
	goMod, err := sh.Output("go", "list", ".")
	if err != nil {
		return "", fmt.Errorf("could not execute go list: %w", err)
	}

	return goMod, nil
}

func getPackagesToTest(moduleName string, excludes []string) ([]string, error) {
	output, err := sh.Output("go", "list", "./...")
	if err != nil {
		return []string{}, fmt.Errorf("could not execute go list: %w", err)
	}

	allPackages := strings.Split(strings.Replace(output, "\r\n", "\n", -1), "\n")
	var filteredPackages []string

	for _, p := range allPackages {
		if packageMatches(p, excludes, moduleName) {
			continue
		}
		filteredPackages = append(filteredPackages, p)
	}
	return filteredPackages, nil
}

func packageMatches(p string, excludes []string, moduleName string) bool {
	for _, exclude := range excludes {
		excludedPackage := fmt.Sprintf("%s/%s", moduleName, exclude)
		if p == excludedPackage {
			return true
		}
	}
	return false
}
