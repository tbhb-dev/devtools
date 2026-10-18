// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command agentrepo provides shared repository tools.
package main

import (
	"os"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func main() {
	os.Exit(agentrepo.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
