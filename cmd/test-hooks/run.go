// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/tbhb-dev/devtools/internal/bats"
)

// Run validates the optional suite path and invokes the runner.
func Run(args []string, out, diagnostic io.Writer) int {
	if len(args) > 1 {
		fmt.Fprintln(diagnostic, "usage: test-hooks [path]")
		return 1
	}
	path := "tests"
	if len(args) == 1 {
		path = args[0]
	}
	return bats.Run(context.Background(), path, os.Getenv("HOOKS_JOBS"), out, diagnostic)
}
