// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command commit-lint checks commit-message shape and attribution trailers.
package main

import (
	"os"

	"github.com/tbhb-dev/devtools/internal/commitlint"
)

func main() {
	// An absolute dot resolves config discovery from the caller's repository.
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	os.Exit(commitlint.Run(os.Args[1:], os.Stdin, os.Stderr, dir, os.Getenv))
}
