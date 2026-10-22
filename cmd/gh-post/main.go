// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command gh-post scans GitHub content before posting it.
package main

import (
	"context"
	"os"

	"github.com/tbhb-dev/devtools/internal/ghpost"
)

func main() {
	os.Exit(ghpost.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
