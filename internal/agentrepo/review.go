// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/review"
)

func reviewCommand(code *int) *cobra.Command {
	group := &cobra.Command{Use: "review", Short: "Run independent reviews"}
	var timeout time.Duration
	run := &cobra.Command{
		Use:   "run --timeout DURATION -- COMMAND [ARGS...]",
		Short: "Bound a reviewer and its process group (macOS and Linux)",
		Long:  "Pass stdin through and wait for command exit and output EOF. Kill the process group on timeout or SIGINT/SIGTERM. Exit codes: 0 success, 1 command failure, 124 timeout, 130 cancellation.",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			*code = review.Run(ctx, timeout, args, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	run.Flags().DurationVar(&timeout, "timeout", 0, "Positive deadline, e.g. 900s (required)")
	group.AddCommand(run, redesignCommand(code))
	return group
}
