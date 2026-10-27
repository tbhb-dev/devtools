// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func prLabelsCommand(code *int) *cobra.Command {
	var available string
	cmd := &cobra.Command{
		Use:   "pr-labels",
		Short: "Validate newline-separated draft labels from stdin against a repository snapshot",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("read draft labels: %w", err)
			}
			known := make(map[string]bool)
			for label := range strings.SplitSeq(available, "\n") {
				known[label] = true
			}
			for label := range strings.SplitSeq(string(data), "\n") {
				if label != "" && !known[label] {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "no label named %q in this repository\n", label)
					*code = 1
				}
			}
			if *code != 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Available labels:\n%s\n", available)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&available, "available", "", "Complete newline-separated repository label snapshot")
	return cmd
}
