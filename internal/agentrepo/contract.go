// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/consumercontract"
)

const notesMode = 0o644

func contractCommand(code *int) *cobra.Command {
	group := &cobra.Command{Use: "contract", Short: "Validate, render and apply the consumer configuration contract"}
	var repo string
	group.PersistentFlags().StringVar(&repo, "repo", ".", "Repository tree to judge or update")
	group.AddCommand(
		contractValidate(code, &repo),
		contractRender(code, &repo),
		contractPlan(code, &repo),
		contractApply(code, &repo),
		contractVerify(code, &repo),
	)
	return group
}

func contractFail(cmd *cobra.Command, code *int, err error) {
	*code = 1
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), err)
}

// requireFlags marks flags the command declared a line earlier. Cobra fails
// only for a name that was never declared, which is a programming error.
func requireFlags(cmd *cobra.Command, names ...string) {
	for _, name := range names {
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(err)
		}
	}
}

func contractValidate(code *int, repo *string) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Judge the tree as the next release before anything is written",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			candidate, err := consumercontract.ValidateCandidate(cmd.Context(), *repo)
			if err != nil {
				contractFail(cmd, code, err)
				return
			}
			cmd.Printf(
				"consumer contract valid against %s (%d transitions declared)\n",
				candidate.PreviousTag,
				len(candidate.Document.Transition.Entries),
			)
		},
	}
}

func contractRender(code *int, repo *string) *cobra.Command {
	var check bool
	render := &cobra.Command{
		Use:   "render",
		Short: "Write the consumer notes from the contract, or check they are current",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			if err := renderNotes(*repo, check); err != nil {
				contractFail(cmd, code, err)
			}
		},
	}
	render.Flags().BoolVar(&check, "check", false, "Fail when the notes differ instead of writing them")
	return render
}

// renderNotes writes the notes the contract in repo renders, or with check
// reports whether the notes already there are those.
func renderNotes(repo string, check bool) error {
	data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(consumercontract.ContractPath)))
	if err != nil {
		return fmt.Errorf("read contract: %w", err)
	}
	doc, err := consumercontract.Parse(data)
	if err != nil {
		return fmt.Errorf("parse contract: %w", err)
	}
	rendered := consumercontract.Render(doc)
	notes := filepath.Join(repo, filepath.FromSlash(consumercontract.NotesPath))
	if check {
		current, readErr := os.ReadFile(notes)
		if readErr != nil || string(current) != rendered {
			return fmt.Errorf("%w: run `mise run generate-consumer-notes`", consumercontract.ErrNotesStale)
		}
		return nil
	}
	//nolint:gosec // The notes path comes from the repository flag and the notes are world-readable documentation.
	if err = os.WriteFile(notes, []byte(rendered), notesMode); err != nil {
		return fmt.Errorf("write notes: %w", err)
	}
	return nil
}

func contractPlan(code *int, repo *string) *cobra.Command {
	var remote, tag, state string
	plan := &cobra.Command{
		Use:   "plan",
		Short: "Decide before any edit whether the tree can move to a tag, and what that changes",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			result, err := consumercontract.Prepare(cmd.Context(), *repo, remote, tag, state)
			if err != nil {
				contractFail(cmd, code, err)
				return
			}
			if len(result.Actions) == 0 {
				cmd.Printf("no consumer transition applies for %s\n", tag)
				return
			}
			for _, entry := range result.Actions {
				cmd.Printf("transition: %s\n", entry.ID())
			}
		},
	}
	plan.Flags().StringVar(&remote, "remote", "", "Repository to fetch the target tag from")
	plan.Flags().StringVar(&tag, "tag", "", "Release tag to move to")
	plan.Flags().StringVar(&state, "state", "", "Scratch directory shared with apply and verify")
	requireFlags(plan, "remote", "tag", "state")
	return plan
}

func contractApply(code *int, repo *string) *cobra.Command {
	var state string
	apply := &cobra.Command{
		Use:   "apply",
		Short: "Write the planned transitions into the tree and nothing else",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			changed, err := consumercontract.Apply(*repo, state)
			if err != nil {
				contractFail(cmd, code, err)
				return
			}
			if changed {
				cmd.Printf("updated %s\n", consumercontract.ConfFile)
			}
		},
	}
	apply.Flags().StringVar(&state, "state", "", "Scratch directory plan wrote")
	requireFlags(apply, "state")
	return apply
}

func contractVerify(code *int, repo *string) *cobra.Command {
	var state string
	verify := &cobra.Command{
		Use:   "verify",
		Short: "Run the source and target readers over the tree as the bump left it",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			if err := consumercontract.Verify(cmd.Context(), *repo, state); err != nil {
				contractFail(cmd, code, err)
			}
		},
	}
	verify.Flags().StringVar(&state, "state", "", "Scratch directory plan wrote")
	requireFlags(verify, "state")
	return verify
}
