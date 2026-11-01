// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// This command runs a command with isolated test configuration.
package main

import (
	"os"

	"github.com/tbhb-dev/devtools/internal/testenv"
)

func main() {
	os.Exit(testenv.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
