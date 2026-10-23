// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command agy-otel records Antigravity hook events in a local spool.
package main

import "os"

func main() {
	Run(os.Args[1:], os.Stdin)
}
