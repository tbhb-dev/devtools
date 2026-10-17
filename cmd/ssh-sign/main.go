// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command ssh-sign supplies Git's SSH signing command with an isolated signing agent.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tbhb-dev/devtools/internal/signing"
)

func main() {
	home, err := os.UserHomeDir()
	if err == nil {
		err = signing.Run(context.Background(), home, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ssh-sign:", err)
		os.Exit(1)
	}
}
