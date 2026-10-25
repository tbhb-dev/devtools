// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/mergepolicy"
)

func mergeCommand(code *int) *cobra.Command {
	group := &cobra.Command{Use: "merge", Short: "Evaluate saved merge policy inputs"}
	var file, login, head, authorType string
	approval := &cobra.Command{
		Use:   "approval",
		Short: "Require the latest decisive review to approve the exact head",
		Long:  "Read a saved review snapshot without network access. Exit codes: 0 qualifying approval, 1 no qualifying approval, 5 invalid input.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			f, err := os.Open(file)
			if err != nil {
				return fmt.Errorf("evaluate merge approval: %w", err)
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			if err != nil {
				return fmt.Errorf("evaluate merge approval: %w", err)
			}
			approved, err := mergepolicy.Approved(data, login, head, authorType)
			if err != nil {
				return fmt.Errorf("evaluate merge approval: %w", err)
			}
			if !approved {
				*code = 1
			}
			return nil
		},
	}
	approval.Flags().StringVar(&file, "file", "", "Saved review snapshot JSON file")
	approval.Flags().StringVar(&login, "login", "", "Machine-selected GraphQL login")
	approval.Flags().StringVar(&head, "head", "", "Exact expected head OID")
	approval.Flags().StringVar(&authorType, "author-type", "", "Required GraphQL author type: Bot or User")
	for _, flag := range []string{"file", "login", "head"} {
		if err := approval.MarkFlagRequired(flag); err != nil {
			panic(err)
		}
	}
	group.AddCommand(approval)
	return group
}
