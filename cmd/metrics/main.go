// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command metrics records events in a local journal.
package main

import (
	"os"

	"github.com/tbhb-dev/devtools/internal/metrics"
)

func main() {
	os.Exit(metrics.Run(os.Args[1:], os.Stdout, os.Stderr))
}
