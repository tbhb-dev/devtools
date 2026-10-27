// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

func fuzzCommand(code *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "fuzz-go [PATH]",
		Short:              "Run native Go fuzz targets sequentially (FUZZ_TIME defaults to 30s)",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 1 && args[0] == "--supported" {
				return
			}
			path := "./..."
			if len(args) > 0 && args[0] != "" {
				path = args[0]
			}
			duration := os.Getenv("FUZZ_TIME")
			if duration == "" {
				duration = "30s"
			}
			*code = runFuzz(cmd, path, duration)
		},
	}
	// Declare the capability in help while leaving path and extra arguments
	// untouched, as the shell's first positional argument contract requires.
	cmd.Flags().Bool("supported", false, "Probe native fuzz orchestration without running Go")
	return cmd
}

func runFuzz(cmd *cobra.Command, path, duration string) int {
	list := exec.CommandContext(cmd.Context(), "go", "list", path)
	list.Stderr = cmd.ErrOrStderr()
	packages := fuzzListing(list)
	found := false
	for pkg := range strings.FieldsSeq(packages) {
		discover := exec.CommandContext(cmd.Context(), "go", "test", "-list", "^Fuzz", pkg)
		discover.Stderr = io.Discard
		targets := fuzzListing(discover)
		for target := range strings.SplitSeq(strings.TrimRight(targets, "\n"), "\n") {
			if !strings.HasPrefix(target, "Fuzz") {
				continue
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "==> %s %s\n", pkg, target)
			run := exec.CommandContext(cmd.Context(), "go")
			run.Args = append(run.Args, "test", "-run=^$", "-fuzz=^"+target+"$", "-fuzztime="+duration, pkg)
			run.Stdin, run.Stdout, run.Stderr = cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
			if err := run.Run(); err != nil {
				return fuzzExit(err, cmd.ErrOrStderr())
			}
			found = true
		}
	}
	if !found {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "no fuzz targets discovered under %s\n", path)
		return 1
	}
	return 0
}

func fuzzExit(err error, stderr io.Writer) int {
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		if status, isWaitStatus := exit.Sys().(syscall.WaitStatus); isWaitStatus && status.Signaled() {
			const shellSignalBase = 128
			return shellSignalBase + int(status.Signal())
		}
		return exit.ExitCode()
	}
	_, _ = fmt.Fprintln(stderr, err)
	const commandNotFound = 127
	return commandNotFound
}

// fuzzListing preserves the shell's partial-output behavior on discovery errors.
func fuzzListing(cmd *exec.Cmd) string {
	var output bytes.Buffer
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		// Keep targets and packages already printed.
		return output.String()
	}
	return output.String()
}
