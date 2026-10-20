// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package reviewer

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func execGH(binary string, args, env []string) error {
	// Windows lacks process replacement. Inherit streams and preserve the CLI exit code.
	cmd := exec.Command(binary, args[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		return fmt.Errorf("run gh: %w", err)
	}
	return nil
}
