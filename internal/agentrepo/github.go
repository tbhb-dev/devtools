// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"slices"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

const githubAPI = "api"

func githubCommand(code *int) *cobra.Command {
	group := &cobra.Command{Use: "github", Short: "Run GitHub CLI calls with quota diagnostics"}
	group.AddCommand(
		&cobra.Command{
			Use:                "run [gh arguments...]",
			Short:              "Run gh once, preserving streams and failure status",
			DisableFlagParsing: true,
			Run:                func(cmd *cobra.Command, args []string) { *code = runGitHub(cmd, args, cmd.OutOrStdout()) },
		},
		&cobra.Command{
			Use: "login", Short: "Read the active GitHub login", Args: cobra.NoArgs,
			Run: func(cmd *cobra.Command, _ []string) {
				var out bytes.Buffer
				*code = runGitHub(
					cmd,
					[]string{
						githubAPI,
						"graphql",
						"-f",
						"query={viewer{login}}",
						"--jq",
						".data.viewer.login // empty",
					},
					&out,
				)
				if *code != 0 {
					return
				}
				login := strings.TrimRight(out.String(), "\n")
				if login == "" || login == "null" {
					cmd.PrintErrln("GitHub returned no viewer login")
					*code = 1
					return
				}
				cmd.Println(login)
			},
		},
		&cobra.Command{
			Use:                "repo query [gh arguments...]",
			Short:              "Read the current repository through GraphQL",
			DisableFlagParsing: true,
			Run: func(cmd *cobra.Command, args []string) {
				if len(args) == 0 {
					cmd.PrintErrln("usage: agentrepo github repo query [gh arguments...]")
					*code = 1
					return
				}
				argv := append(
					[]string{
						githubAPI,
						"graphql",
						"-F",
						"owner={owner}",
						"-F",
						"name={repo}",
						"-f",
						"query=" + args[0],
					},
					args[1:]...,
				)
				*code = runGitHub(cmd, argv, cmd.OutOrStdout())
			},
		},
	)
	return group
}

func runGitHub(cmd *cobra.Command, args []string, out io.Writer) int {
	gh := exec.CommandContext(cmd.Context(), "gh", args...)
	var stderr bytes.Buffer
	gh.Stdin, gh.Stdout, gh.Stderr = cmd.InOrStdin(), out, &stderr
	err := gh.Run()
	status := 0
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			status = githubExitStatus(exit)
		} else {
			cmd.PrintErrln(err)
			return 1
		}
		if guidance := githubQuota(args, stderr.String()); guidance != "" {
			cmd.PrintErrln(guidance + " No automatic retry was made.")
			status = 76
		}
	}
	if _, writeErr := cmd.ErrOrStderr().Write(stderr.Bytes()); writeErr != nil {
		return 1
	}
	return status
}

func githubExitStatus(exit *exec.ExitError) int {
	const signalExitBase = 128
	if wait, ok := exit.Sys().(syscall.WaitStatus); ok && wait.Signaled() {
		return signalExitBase + int(wait.Signal())
	}
	return exit.ExitCode()
}

func githubQuota(args []string, stderr string) string {
	for line := range strings.SplitSeq(stderr, "\n") {
		graphql := strings.HasPrefix(line, "GraphQL: ")
		if !graphql && !strings.HasPrefix(line, "gh: ") {
			continue
		}
		if strings.Contains(line, "secondary rate limit") {
			return "rate limit: GitHub secondary rate limit; the reset time isn't known from the failure. Wait at least one minute before the next request, as GitHub's docs advise; no budget query helps."
		}
		if !strings.Contains(line, "API rate limit exceeded for") &&
			!strings.Contains(line, "API rate limit already exceeded for") {
			continue
		}
		if len(args) == 0 || args[0] != githubAPI || slices.Contains(args, "graphql") {
			graphql = true
		}
		if graphql {
			return "rate limit: GitHub primary GraphQL rate limit, inferred from the command and the error line; the reset time isn't known from the failure. Once the limit has passed, read the budget with a GraphQL rateLimit { remaining resetAt } query, never /rate_limit."
		}
		return "rate limit: GitHub primary REST rate limit, inferred from the command and the error line; the reset time isn't known from the failure. The budget can't be read reliably from here: wait for the hourly window and treat /rate_limit as advisory only."
	}
	return ""
}
