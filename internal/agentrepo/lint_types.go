// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func lintTypesCommand(code *int) *cobra.Command {
	return &cobra.Command{
		Use:   "lint-types",
		Short: "Run uv run pyrefly check with the existing worktree exemption",
		Long: "Run uv run pyrefly check with LC_ALL=C. Extra arguments are ignored. " +
			"This additive command leaves consumer tasks on the existing shell adapter. " +
			"Use agentrepo help lint-types to display this help.",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, _ []string) {
			child := exec.CommandContext(cmd.Context(), "uv", "run", "pyrefly", "check")
			child.Env = append(os.Environ(), "LC_ALL=C")
			child.Stdin = cmd.InOrStdin()
			output, err := child.CombinedOutput()
			status := 0
			if err != nil {
				if errors.Is(err, exec.ErrNotFound) {
					// Preserve the frozen adapter's Bash diagnostic, including its source location.
					_, _ = fmt.Fprintln(
						cmd.OutOrStdout(),
						".devtools/scripts/lint-types.sh: line 23: uv: command not found",
					)
					*code = 127
					return
				}
				status = fuzzExit(err, cmd.OutOrStdout())
			}
			// The current directory retains valid logical spelling through symlinks, like Bash.
			directory, cwdErr := os.Getwd()
			if status != 0 && cwdErr == nil && strings.Contains(directory, "/.claude/worktrees/") &&
				strings.Contains(string(output), "No Python files matched patterns") {
				_, _ = fmt.Fprintln(
					cmd.OutOrStdout(),
					"lint-types: pyrefly matched no files behind the worktree's hidden path; skipping — CI checks the real tree",
				)
				return
			}
			// Command substitution removes every trailing newline. Printing adds exactly one.
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), strings.TrimRight(string(output), "\n"))
			*code = status
		},
	}
}
