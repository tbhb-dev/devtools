// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command shell-allowlist reports commands outside the repository's shell allow list.
package main

import "os"

func main() {
	os.Exit(Run(os.Args[1:], os.Stderr))
}
