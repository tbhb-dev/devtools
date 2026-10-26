// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/contentposts"
)

func contentPostsCommand(code *int) *cobra.Command {
	group := &cobra.Command{Use: "content-posts", Short: "Find content writes that bypass gh-post"}
	group.AddCommand(&cobra.Command{
		Use:   "scan [file ...]",
		Short: "Scan Bash files; default to every .sh under packages/agents/source",
		RunE: func(cmd *cobra.Command, paths []string) error {
			if len(paths) == 0 {
				var err error
				paths, err = contentposts.Scripts(".")
				if err != nil {
					*code = 2
					cmd.PrintErrln(fmt.Errorf("discover skill scripts: %w", err))
					return nil
				}
			}
			*code = contentposts.Scan(paths, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	})
	return group
}
