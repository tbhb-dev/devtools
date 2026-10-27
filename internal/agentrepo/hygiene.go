// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/hygiene"
)

func hygieneCommand(code *int) *cobra.Command {
	return &cobra.Command{
		Use:                "check-script-hygiene [file ...]",
		Short:              "Check shell environment hardening and structural hygiene",
		Long:               "Check shell scripts, or report capability with --probe without reading the repository.",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 1 && args[0] == "--probe" {
				cmd.Println("script-hygiene-v1")
				return
			}
			*code = hygiene.Run(cmd.Context(), "", args, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}
