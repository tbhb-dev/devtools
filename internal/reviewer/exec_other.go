// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Keep this fallback visible when Linux resolves merged cross-platform coverage.

package reviewer

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func execOther(binary string, args, env []string) error {
	// Windows lacks process replacement. Inherit streams and preserve the CLI exit code.
	//nolint:gosec,noctx // Execute the resolved CLI with its own lifetime and unchanged arguments.
	cmd := exec.Command(binary, args[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		return fmt.Errorf("run gh: %w", err)
	}
	return nil
}
