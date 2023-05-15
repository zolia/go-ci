/*
 * Copyright (c) 2021. Monimoto, UAB. All Rights Reserved.
 *
 * This software contains the intellectual property of Monimoto, UAB. Use of this software and the intellectual
 * property contained therein is expressly limited to the terms and conditions of the License Agreement under
 * which it is provided by or on behalf of Monimoto, UAB.
 */

package deploy

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/joho/godotenv"
	"github.com/magefile/mage/sh"
)

// ValidateEnv validates and prints local and remote .env file diffs
func ValidateEnv(homeDir string, sshArgs []string, envFile string, print bool) (bool, error) {
	output, err := sh.Output("ssh", append(sshArgs, "cd "+homeDir+" && cat .env")...)
	if err != nil {
		return false, err
	}

	buf := strings.NewReader(output)
	remoteEnv, err := godotenv.Parse(buf)
	if err != nil {
		return false, err
	}
	localFile, err := os.ReadFile(envFile)
	if err != nil {
		return false, err
	}

	localEnv, err := godotenv.Parse(strings.NewReader(string(localFile)))
	if err != nil {
		return false, err
	}

	var envsAreIdentical = diffEnvs(localEnv, remoteEnv, envFile, print)
	return envsAreIdentical, nil
}

type env map[string]string

func diffEnvs(localEnv, remoteEnv env, envFile string, print bool) bool {
	var (
		remoteMissing  = make(env, 0)
		remoteMismatch = make(env, 0)
		localMissing   = make(env, 0)
		localMismatch  = make(env, 0)
	)

	for name, value := range localEnv {
		remoteValue, ok := remoteEnv[name]
		if !ok {
			remoteMissing[name] = value
			continue
		}

		if remoteValue != value {
			remoteMismatch[name] = value
		}
	}

	for name, value := range remoteEnv {
		localValue, ok := localEnv[name]
		if !ok {
			localMissing[name] = value
			continue
		}

		if localValue != value {
			localMismatch[name] = value
		}
	}

	diffCount := len(localMissing) + len(localMismatch) + len(remoteMissing) + len(remoteMismatch)

	if print && diffCount > 0 {
		fmt.Printf("Error: .env file mismatch. Please update your %s file or the .env file remotely.\n", envFile)

		if len(localMissing) > 0 || len(localMismatch) > 0 {
			fmt.Println()
			printDiff(fmt.Sprintf("add to local %s or remove from remote .env", envFile), localMissing, "<")
			printDiff(fmt.Sprintf("update local %s", envFile), localMismatch, "<")
			fmt.Println()
		}

		fmt.Println("or")
		fmt.Println()
		if len(remoteMissing) > 0 || len(remoteMismatch) > 0 {
			printDiff(fmt.Sprintf("add to remote .env or remove from local .env"), remoteMissing, ">")
			printDiff(fmt.Sprintf("update remote .env"), remoteMismatch, ">")
		}
		fmt.Println()
	}

	return diffCount == 0
}

func printDiff(title string, diff env, sign string) {
	if len(diff) == 0 {
		return
	}

	fmt.Println(title)

	keys := make([]string, 0, len(diff))
	for k := range diff {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, v := range keys {
		fmt.Printf("%s %s=%v\n", sign, v, diff[v])
	}
}
