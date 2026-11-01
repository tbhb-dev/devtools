// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/retro"
)

func retroCommand(code *int) *cobra.Command {
	group := &cobra.Command{Use: "retro", Short: "Scaffold and profile session retrospectives"}
	var skill string
	group.PersistentFlags().
		StringVar(&skill, "skill-dir", ".", "Deployed analyze-session directory containing scripts/ and templates/")
	group.AddCommand(
		&cobra.Command{
			Use:   "new <transcript.jsonl> [extra-artifact ...]",
			Short: "Preserve session evidence under the main checkout",
			Run: func(cmd *cobra.Command, args []string) {
				*code = retro.New(cmd.Context(), skill, args, cmd.OutOrStdout(), cmd.ErrOrStderr())
			},
		},
	)
	return group
}
