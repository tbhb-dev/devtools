// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/versions"
)

func versionsCommand(code *int) *cobra.Command {
	return &cobra.Command{
		Use:   "check-versions [VERSION]",
		Short: "Check published release literals and local locks against the nearest tag or VERSION",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			version := ""
			if len(args) != 0 {
				version = args[0]
			}
			*code = versions.Run(cmd.Context(), "", version, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}
