// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/config"
)

func configCommand(code *int) *cobra.Command {
	const maxArgs = 2
	group := &cobra.Command{Use: "config", Short: "Read machine authority or repository behavior settings"}
	for _, kind := range []string{"machine", "merge-machine", "repository"} {
		group.AddCommand(&cobra.Command{
			Use:   kind + " KEY [DEFAULT]",
			Short: "Print a literal " + kind + " setting",
			Args:  cobra.RangeArgs(1, maxArgs),
			Run: func(cmd *cobra.Command, args []string) {
				lookup := config.Lookup{
					LookupEnv: os.LookupEnv,
					RepositoryName: func() (string, error) {
						common, err := configGit(cmd, "--path-format=absolute", "--git-common-dir")
						return filepath.Base(filepath.Dir(common)), err
					},
					RepositoryRoot: func() (string, error) { return configGit(cmd, "--show-toplevel") },
				}
				fallback := ""
				if len(args) == maxArgs {
					fallback = args[1]
				}
				read := lookup.Machine
				switch kind {
				case "repository":
					read = lookup.Repository
				case "merge-machine":
					read = lookup.MergeMachine
				}
				value, err := read(args[0], fallback)
				if err != nil {
					*code = 1
					if exit, ok := errors.AsType[*exec.ExitError](err); ok {
						*code = exit.ExitCode()
					} else {
						_, _ = fmt.Fprintln(cmd.ErrOrStderr(), err)
					}
					return
				}
				cmd.Println(value)
			},
		})
	}
	return group
}

func configGit(cmd *cobra.Command, args ...string) (string, error) {
	// Only fixed rev-parse arguments from this command are passed to Git.
	//nolint:gosec // No user-controlled executable or arguments.
	git := exec.CommandContext(cmd.Context(), "git", append([]string{"--no-pager", "rev-parse"}, args...)...)
	git.Stderr = cmd.ErrOrStderr()
	data, err := git.Output()
	if err != nil {
		return "", fmt.Errorf("git context: %w", err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}
