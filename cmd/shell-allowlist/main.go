// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command shell-allowlist reports the commands in shell scripts that the
// repository's allow list doesn't permit.
//
// It checks exactly the files named on its command line. Finding the shell
// files in a tree is the caller's job.
//
// See Run in run.go.
package main

import "os"

func main() {
	os.Exit(Run(os.Args[1:], streams{out: os.Stdout, err: os.Stderr}))
}
