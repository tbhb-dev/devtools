// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command test-affected runs the repository tests selected by the Git diff.
package main

import (
	"context"
	"os"

	"github.com/tbhb-dev/devtools/internal/testaffected"
)

func main() {
	os.Exit(testaffected.Run(context.Background(), os.Stdout, os.Stderr))
}
