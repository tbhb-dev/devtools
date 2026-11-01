// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command guard-workflow judges hook commands for the four gh-family skills.
package main

import "os"

func main() {
	os.Exit(Run(os.Args[1:], streams{in: os.Stdin, out: os.Stdout, err: os.Stderr}))
}
