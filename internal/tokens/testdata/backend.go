// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Deterministic executable used as git, uv, and ant on every native runner.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if name == "git" {
		if os.Getenv("TOKEN_TEST_GIT_FAIL") != "" {
			os.Exit(23)
		}
		if os.Args[1] == "rev-parse" {
			fmt.Println(os.Getenv("TOKEN_TEST_ROOT"))
		}
		return
	}
	data, _ := io.ReadAll(os.Stdin)
	if os.Getenv("TOKEN_TEST_FAIL") != "" {
		fmt.Println(10)
		os.Exit(23)
	}
	if name == "ant" {
		fmt.Println(`{"input_tokens":17}`)
	} else {
		fmt.Println(len(data))
	}
}
