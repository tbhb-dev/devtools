// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package signing

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// Run handles signing and the explicit, operator-run installer.
func Run(ctx context.Context, home string, args []string, in io.Reader, out, stderr io.Writer) error {
	config, err := ReadConfig(home, os.Getenv)
	if err != nil {
		return err
	}
	if len(args) == 1 && args[0] == "--install" {
		binary, binaryErr := os.Executable()
		if binaryErr != nil {
			return fmt.Errorf("binary path: %w", binaryErr)
		}
		sshAgent, lookupErr := exec.LookPath("ssh-agent")
		if lookupErr != nil {
			return fmt.Errorf("ssh-agent path: %w", lookupErr)
		}
		return Install(home, binary, sshAgent, config)
	}
	agent := Agent{Config: config, Now: time.Now}
	return agent.Sign(ctx, args, in, out, stderr)
}
