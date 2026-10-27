// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/apmsync"
)

func apmCommand(code *int) *cobra.Command {
	return &cobra.Command{
		Use:   "apm-sync",
		Short: "Clear obsolete APM hook settings and install primitives",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			*code = apmsync.Run(cmd.Context(), "", cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}
