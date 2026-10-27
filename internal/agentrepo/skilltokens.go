// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/tokens"
)

func skillTokensCommand(code *int) *cobra.Command {
	return &cobra.Command{
		Use:   "skill-tokens [skill-dir ...]",
		Short: "Measure skill token costs and write compatible token records",
		Run: func(cmd *cobra.Command, args []string) {
			*code = tokens.Run(cmd.Context(), args, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}
