// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command prctl manages pull requests through the skill entry points.
package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	if err := run(
		context.Background(),
		os.Args[1:],
		githubCLI{},
		os.Getenv("DEVTOOLS_PR_CLOSE_GRANTED") == "1",
	); err != nil {
		fmt.Fprintln(os.Stderr, "prctl:", err)
		os.Exit(1)
	}
}
