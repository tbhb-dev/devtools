// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command test-hooks runs the Bats suite with bounded file parallelism.
package main

import "os"

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}
