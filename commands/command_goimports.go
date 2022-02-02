/*
 * Copyright (C) 2018 The "MysteriumNetwork/go-ci" Authors.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/zolia/go-ci/shell"
	"github.com/zolia/go-ci/util"
)

// GetImports Fetches the goimports binary
func GetImports() error {
	path, _ := util.GetGoBinaryPath("goimports")
	if path != "" {
		fmt.Println("Tool 'goimports' already installed")
		return nil
	}

	cmd := shell.NewCmd("$GOROOT/bin/go get -u -x -v golang.org/x/tools/cmd/goimports")
	out, err := cmd.Output()
	fmt.Printf("go get goimports output: %s\n", out)
	if err != nil {
		fmt.Printf("Could not go get goimports: %s\n", err)
		return err
	}
	return nil
}

// GoImportsD checks for issues with go imports.
//
// Instead of packages, it operates on directories, thus it is compatible with gomodules outside GOPATH.
//
// Example:
//     commands.GoImportsD(".", "docs")
func GoImportsD(dir string, excludes ...string) error {
	mg.Deps(GetImports)
	goimportsBin, err := util.GetGoBinaryPath("goimports")
	if err != nil {
		fmt.Println("Tool 'goimports' not found")
		return err
	}
	var allExcludes []string
	allExcludes = append(allExcludes, excludes...)
	allExcludes = append(allExcludes, util.GoLintExcludes()...)
	dirs, err := util.GetProjectFileDirectories(allExcludes)
	if err != nil {
		return err
	}
	out, err := shell.NewCmd(goimportsBin + " -e -l -d " + strings.Join(dirs, " ")).Output()
	if err != nil {
		fmt.Printf("goimports: error executing %s\n", err)
		return err
	}
	if len(out) != 0 {
		fmt.Println("goimports: the following files contain go import errors:")
		fmt.Println(out)
		return errors.New("goimports: not all imports follow the goimports format")
	}
	fmt.Println("goimports: all files are OK!")
	return nil
}
