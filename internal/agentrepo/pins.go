// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/pins"
)

func pinsCommand(code *int) *cobra.Command {
	return &cobra.Command{
		Use:   "update-pins VERSION",
		Short: "Move consumer devtools pins to a release and refresh the payload",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			*code = pins.Run(cmd.Context(), "", args[0], cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}
