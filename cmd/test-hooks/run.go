// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"context"
	"io"
	"os"

	"github.com/tbhb-dev/devtools/internal/bats"
)

// Run defaults the optional suite paths and invokes the runner.
func Run(args []string, out, diagnostic io.Writer) int {
	if len(args) == 0 {
		args = []string{"tests"}
	}
	return bats.Run(context.Background(), args, os.Getenv("HOOKS_JOBS"), out, diagnostic)
}
