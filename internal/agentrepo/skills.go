// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/skills"
)

func skillsCommand(code *int) *cobra.Command {
	return &cobra.Command{
		Use:                "generate-skills [--check]",
		Short:              "Generate harness skills from canonical sources",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 1 && args[0] == "--capability" {
				cmd.Println("generate-skills-v1")
				return
			}
			check := false
			if len(args) > 0 {
				switch args[0] {
				case "":
				case "--check":
					check = true
				default:
					cmd.PrintErrln("usage: generate-skills.sh [--check]")
					*code = 2
					return
				}
			}
			git := exec.CommandContext(cmd.Context(), "git", "rev-parse", "--show-toplevel")
			git.Stderr = cmd.ErrOrStderr()
			data, err := git.Output()
			if err != nil {
				if exit, ok := errors.AsType[*exec.ExitError](err); ok {
					*code = exit.ExitCode()
					return
				}
				fmt.Fprintln(cmd.ErrOrStderr(), err)
				*code = 1
				return
			}
			*code = skills.Run(strings.TrimRight(string(data), "\r\n"), check, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}
