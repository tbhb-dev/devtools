// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package agentrepo provides the agentrepo command tree.
package agentrepo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/buildmeta"
	"github.com/tbhb-dev/devtools/internal/checks"
	"github.com/tbhb-dev/devtools/internal/prsize"
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
	var file, head, base, mergeBase string
	var number int
	var required []string
	evaluate := &cobra.Command{
		Use:   "evaluate",
		Short: "Evaluate a saved rollup and print a JSON verdict",
		Long:  "Read gh pr view JSON from --file or stdin. Exit codes: 0 success, 1 failed, 2 pending, 3 missing required, 4 empty, 5 unreadable, 6 head moved. Required names match exactly. With --pr, a failure or a missing required check beside a required check that concluded SKIPPED also prints the update-branch hint to stderr, using --base and --merge-base; the exit status is unchanged.",
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
			if number > 0 {
				for _, line := range checks.Hint(result, number, base, mergeBase) {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), line)
				}
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	evaluate.Flags().StringVar(&file, "file", "-", "Saved JSON file, or - for stdin")
	evaluate.Flags().StringVar(&head, "expected-head", "", "Expected headRefOid")
	evaluate.Flags().StringArrayVar(&required, "required", nil, "Required check name (repeatable)")
	evaluate.Flags().IntVar(&number, "pr", 0, "Pull request number, enabling the stale merge ref hint")
	evaluate.Flags().StringVar(&base, "base", "", "Base commit the pull request merges into")
	evaluate.Flags().
		StringVar(&mergeBase, "merge-base", "", "Merge base of the base commit and the head; empty when unknown")
	group.AddCommand(evaluate)
	root.AddCommand(
		group,
		reviewCommand(code),
		configCommand(code),
		markersCommand(code),
		versionsCommand(code),
		fuzzCommand(code),
	)
	size := sizeCommand()
	size.AddCommand(sizeCheckCommand())
	root.AddCommand(size)
	return root, code
}

var errSizeFormat = errors.New("size format must be text or json")

const sizeError = 2

func sizeCommand() *cobra.Command {
	var base, head, format string
	cmd := &cobra.Command{
		Use:   "size",
		Short: "Measure changed code and prose SLOC from the merge base",
		Long:  "Measure added and removed code with scc and nonblank prose lines. Excludes lockfiles, deployed skills, vendor, fixtures and CHANGELOG.md; .devtools is excluded in consumers. Limits: soft above 400 code or 1000 prose, hard above 800 code or 2000 prose. Levels exit 0; usage and environment errors exit 2. Hunk fragments lose surrounding comment context.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if format != "text" && format != "json" {
				return fmt.Errorf("%w: %q", errSizeFormat, format)
			}
			r, err := prsize.Measure(cmd.Context(), "", base, head)
			if err != nil {
				return fmt.Errorf("size: %w", err)
			}
			if format == "json" {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(r)
			}
			cmd.Printf(
				"code: %d\nprose: %d\nlevel: %s\nsoft: %d code / %d prose\nhard: %d code / %d prose\n",
				r.Code,
				r.Prose,
				r.Level,
				r.Soft.Code,
				r.Soft.Prose,
				r.Hard.Code,
				r.Hard.Prose,
			)
			for _, excluded := range r.Excluded {
				cmd.Printf("excluded: %s (%d raw lines)\n", excluded.Path, excluded.Raw)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Base ref (default origin/<default branch>)")
	cmd.Flags().StringVar(&head, "head", "HEAD", "Head ref")
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text or json")
	return cmd
}

// Run executes the command and returns its process exit code.
func Run(args []string, in io.Reader, out, stderr io.Writer) int {
	cmd, code := NewCommand()
	cmd.SetArgs(args)
	cmd.SetIn(in)
	cmd.SetOut(out)
	cmd.SetErr(stderr)
	if executed, err := cmd.ExecuteC(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		if executed != nil && (executed.Name() == "size" || executed.CommandPath() == "agentrepo size check") {
			if errors.Is(err, errSizeLimit) {
				return 1
			}
			return sizeError
		}
		return int(checks.Unreadable)
	}
	return *code
}
