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

const configRepository = "repository"

func configCommand(code *int) *cobra.Command {
	group := &cobra.Command{Use: "config", Short: "Read machine authority or repository behavior settings"}
	for _, kind := range []string{"machine", "merge-machine", configRepository} {
		group.AddCommand(configSettingCommand(code, kind))
	}
	return group
}

func configSettingCommand(code *int, kind string) *cobra.Command {
	const maxArgs = 2
	var base string
	command := &cobra.Command{
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
			read := configReader(cmd, lookup, kind, base)
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
	}
	if kind == configRepository {
		command.Flags().StringVar(&base, "base", "", "Read settings at the pinned base commit")
	}
	return command
}

func configReader(cmd *cobra.Command, lookup config.Lookup, kind, base string) func(string, string) (string, error) {
	switch kind {
	case "merge-machine":
		return lookup.MergeMachine
	case configRepository:
		if base == "" {
			return lookup.Repository
		}
		return func(key, fallback string) (string, error) {
			value, differs, err := lookup.RepositoryAt(cmd.Context(), base, key, fallback)
			if err != nil {
				return "", fmt.Errorf("pinned configuration: %w", err)
			}
			if differs {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
					"Merge configuration differs from pinned base; ignoring checkout .agents/agent-skills.conf")
			}
			return value, nil
		}
	default:
		return lookup.Machine
	}
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
