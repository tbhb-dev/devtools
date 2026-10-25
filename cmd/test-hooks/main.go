// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command test-hooks runs every hook test file with bounded parallelism.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

func main() {
	err := runSuite(".", runtime.NumCPU(), serialFiles(), func(args []string) error {
		cmd := exec.CommandContext(context.Background(), "bats", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
