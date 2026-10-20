// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//go:build !windows

package reviewer

import (
	"fmt"
	"syscall"
)

func execGH(binary string, args, env []string) error {
	if err := syscall.Exec(binary, args, env); err != nil {
		return fmt.Errorf("replace reviewer process: %w", err)
	}
	return nil
}
