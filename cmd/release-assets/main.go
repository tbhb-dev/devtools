// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command release-assets builds, publishes, and verifies release bundles.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tbhb-dev/devtools/internal/releaseassets"
)

func main() {
	if err := releaseassets.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
