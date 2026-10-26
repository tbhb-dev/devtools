// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"errors"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

// Shells report a signal as 128 plus its number.
const signalExitOffset = 128

func postCommand(code *int) *cobra.Command {
	group := &cobra.Command{Use: "post", Short: "Require and invoke gh-post"}
	group.AddCommand(
		&cobra.Command{
			Use: "require", Short: "Check for gh-post without posting", Args: cobra.NoArgs,
			Run: func(cmd *cobra.Command, _ []string) { requirePost(cmd, code) },
		},
		&cobra.Command{
			Use: "run [gh-post arguments...]", Short: "Pass arguments and streams to gh-post",
			DisableFlagParsing: true,
			Run: func(cmd *cobra.Command, args []string) {
				path := requirePost(cmd, code)
				if path == "" {
					return
				}
				post := exec.CommandContext(cmd.Context(), path, args...)
				post.Stdin, post.Stdout, post.Stderr = cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
				if err := post.Run(); err != nil {
					if exit, ok := errors.AsType[*exec.ExitError](err); ok {
						*code = exit.ExitCode()
						if status, signaled := exit.Sys().(syscall.WaitStatus); signaled && status.Signaled() {
							*code = signalExitOffset + int(status.Signal())
						}
					} else {
						*code = 126
						cmd.PrintErrln(err)
					}
				}
			},
		},
	)
	return group
}

func requirePost(cmd *cobra.Command, code *int) string {
	path, err := exec.LookPath("gh-post")
	if err != nil {
		*code = 2
		cmd.PrintErrln("gh-post is missing; install the gh-post tool before publishing content.")
		return ""
	}
	return path
}
