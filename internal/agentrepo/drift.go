// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/drift"
)

func vendoringCommand(code *int, vendir bool) *cobra.Command {
	name, short := "vendor-check", "Regenerate Go dependencies and check drift against the index"
	if vendir {
		name, short = "check-vendored", "Resync vendir and check payload drift against the index"
	}
	return &cobra.Command{
		Use: name, Short: short, Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			*code = drift.Run(cmd.Context(), vendir, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}
