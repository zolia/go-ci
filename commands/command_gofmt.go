/*
 *  Copyright (c) 2020.  Antanas Masevicius
 *
 *  This source code is licensed under the MIT license found in the
 *  LICENSE file in the root directory of this source tree.
 *
 */

package commands

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"strings"

	"github.com/magefile/mage/sh"

	"github.com/zolia/go-ci/shell"
	"github.com/zolia/go-ci/util"
)

// GoFmt checks for issues with go fmt
func GoFmt(pathToCheck string, excludes ...string) error {
	gofmtBinaryPath, err := util.GetGoBinaryPath("gofmt")
	if err != nil {
		fmt.Println("Tool 'gofmt' not found")
		return err
	}
	gopath := util.GetGoPath()
	dirs, err := util.GetPackagePathsWithExcludes(pathToCheck, excludes...)
	if err != nil {
		fmt.Println("go list crashed")
		return err
	}

	dirsToLook := make([]string, 0)
	for _, dir := range dirs {
		absolutePath := path.Join(gopath, "src", dir)
		res, _ := ioutil.ReadDir(absolutePath)
		for _, v := range res {
			if v.IsDir() {
				continue
			}
			extension := filepath.Ext(v.Name())
			if extension != ".go" {
				continue
			}
			path := path.Join(absolutePath, v.Name())
			dirsToLook = append(dirsToLook, path)
		}
	}

	args := []string{"-e", "-l"}
	args = append(args, dirsToLook...)
	out, err := sh.Output(gofmtBinaryPath, args...)
	if err != nil {
		fmt.Printf("Could not run gofmt: %s\n", err)
		return err
	}
	if len(out) != 0 {
		fmt.Println("The following files contain go fmt errors:")
		fmt.Println(out)
		return errors.New("not all formatting follow the gofmt format")
	}
	fmt.Println("Gofmt is happy - all files are OK!")
	return nil
}

// GoFmtD checks for issues with go formatting.
//
// Instead of packages, it operates on directories, thus it is compatible with gomodules outside GOPATH.
//
// Example:
//     commands.GoFmtD(".", "docs")
func GoFmtD(dir string, excludes ...string) error {
	goFmtBin, err := util.GetGoBinaryPath("gofmt")
	if err != nil {
		fmt.Println("Tool 'gofmt' not found")
		return err
	}
	var allExcludes []string
	allExcludes = append(allExcludes, excludes...)
	allExcludes = append(allExcludes, util.GoLintExcludes()...)
	dirs, err := util.GetProjectFileDirectories(allExcludes)
	if err != nil {
		return err
	}
	out, err := shell.NewCmd(goFmtBin + " -e -l -d " + strings.Join(dirs, " ")).Output()
	if err != nil {
		fmt.Printf("gofmt: error executing %s\n", err)
		return err
	}
	if len(out) != 0 {
		fmt.Println("gofmt: the following files contain go fmt errors:")
		fmt.Println(out)
		return errors.New("gofmt: not all formatting follow the gofmt format")
	}
	fmt.Println("gofmt: all files are OK!")
	return nil
}
