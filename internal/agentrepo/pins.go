// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/pins"
)

func pinsCommand(code *int) *cobra.Command {
	return &cobra.Command{
		Use:   "check-pins",
		Short: "Report consumer devtools pins and fail on disagreement or stale prose",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			*code = pins.Run(cmd.Context(), "", cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}
