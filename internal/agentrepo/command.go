// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package agentrepo provides the agentrepo command tree.
package agentrepo

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/buildmeta"
	"github.com/tbhb-dev/devtools/internal/checks"
)

// NewCommand creates the command and its evaluation exit status.
func NewCommand() (*cobra.Command, *int) {
	code := new(int)
	root := &cobra.Command{
		Use:           "agentrepo",
		Short:         "Shared agent tool for tbhb repositories",
		Version:       buildmeta.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		&cobra.Command{
			Use:   "version",
			Short: "Print version, commit, and build date",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, _ []string) {
				info := buildmeta.Get()
				cmd.Printf("agentrepo %s\ncommit: %s\ndate:   %s\n", info.Version, info.Commit, info.Date)
			},
		},
	)
	group := &cobra.Command{Use: "checks", Short: "Evaluate saved GitHub checks"}
	var file, head string
	var required []string
	evaluate := &cobra.Command{
		Use:   "evaluate",
		Short: "Evaluate a saved rollup and print a JSON verdict",
		Long:  "Read gh pr view JSON from --file or stdin. Exit codes: 0 success, 1 failed, 2 pending, 3 missing required, 4 empty, 5 unreadable, 6 head moved. Required names match exactly.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			input := cmd.InOrStdin()
			if file != "-" {
				f, err := os.Open(file)
				if err != nil {
					*code = int(checks.Unreadable)
					return json.NewEncoder(cmd.OutOrStdout()).
						Encode(checks.Result{Outcome: checks.Unreadable, Error: err.Error()})
				}
				defer f.Close()
				input = f
			}
			data, err := io.ReadAll(input)
			result := checks.Result{Outcome: checks.Unreadable}
			if err != nil {
				result.Error = err.Error()
			} else {
				result = checks.Evaluate(data, head, required)
			}
			*code = int(result.Outcome)
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	evaluate.Flags().StringVar(&file, "file", "-", "Saved JSON file, or - for stdin")
	evaluate.Flags().StringVar(&head, "expected-head", "", "Expected headRefOid")
	evaluate.Flags().StringArrayVar(&required, "required", nil, "Required check name (repeatable)")
	group.AddCommand(evaluate)
	root.AddCommand(group, reviewCommand(code))
	return root, code
}

// Run executes the command and returns its process exit code.
func Run(args []string, in io.Reader, out, stderr io.Writer) int {
	cmd, code := NewCommand()
	cmd.SetArgs(args)
	cmd.SetIn(in)
	cmd.SetOut(out)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return int(checks.Unreadable)
	}
	return *code
}
