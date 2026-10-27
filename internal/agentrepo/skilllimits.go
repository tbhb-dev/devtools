// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/skills"
)

func skillLimitsCommand(code *int) *cobra.Command {
	return &cobra.Command{
		Use:   "check-skill-limits [DIRECTORY ...]",
		Short: "Check offline skill manifest limits",
		Run: func(cmd *cobra.Command, args []string) {
			*code = skills.Run(cmd.Context(), "", args, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}
