// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command gh-reviewer runs GitHub CLI as a repository-scoped App reviewer.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tbhb-dev/devtools/internal/reviewer"
)

func main() {
	home, err := os.UserHomeDir()
	if err == nil {
		err = reviewer.Run(context.Background(), home, os.Args[1:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gh-reviewer:", err)
		os.Exit(1)
	}
}
