// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//go:build !windows

package reviewer

import (
	"fmt"
	"syscall"
)

//nolint:gochecknoinits // Select process replacement before any reviewer command runs.
func init() {
	execGH = execUnix
}

func execUnix(binary string, args, env []string) error {
	// A successful call replaces the process, so this function only returns an error.
	return fmt.Errorf("replace reviewer process: %w", syscall.Exec(binary, args, env))
}
