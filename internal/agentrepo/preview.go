// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/config"
)

func previewCommand(code *int) *cobra.Command {
	// The shell helper historically ignores arguments. Only the standalone
	// capability probe is special, and it must not read identity or run gates.
	return &cobra.Command{
		Use: "commit-preview", Short: "Preview commit-msg gates without changing COMMIT_AGENTMSG",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 1 && args[0] == "--probe" {
				return
			}
			*code = runPreview(cmd)
		},
	}
}

func previewProcess(cmd *cobra.Command, out io.Writer, name string, args ...string) int {
	//nolint:gosec // Executable names and argument shapes come only from fixed preview call sites.
	process := exec.CommandContext(cmd.Context(), name, args...)
	process.Stdin = cmd.InOrStdin()
	process.Stdout = out
	process.Stderr = cmd.ErrOrStderr()
	err := process.Run()
	if err == nil {
		return 0
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return exit.ExitCode()
	}
	cmd.PrintErrln(err)
	const commandNotFound = 127
	return commandNotFound
}

func previewOutput(cmd *cobra.Command, name string, args ...string) (string, int) {
	var out bytes.Buffer
	status := previewProcess(cmd, &out, name, args...)
	return strings.TrimRight(out.String(), "\n"), status
}

func previewIdentity(cmd *cobra.Command) (string, int) {
	lookup := config.Lookup{LookupEnv: os.LookupEnv, RepositoryName: func() (string, error) {
		common, err := configGit(cmd, "--path-format=absolute", "--git-common-dir")
		return filepath.Base(filepath.Dir(common)), err
	}}
	// The frozen helper ignores a failed machine lookup and falls back to Git.
	// An explicitly empty setting also falls back, without checking later files.
	configured, err := lookup.Machine("signoff_identity", "")
	if err != nil {
		configured = ""
	}
	configured = strings.TrimRight(configured, "\n")
	identity := configured
	if identity == "" {
		name, status := previewOutput(cmd, "git", "config", "--get", "user.name")
		if status != 0 {
			name += "UNSET"
		}
		email, status := previewOutput(cmd, "git", "config", "--get", "user.email")
		if status != 0 {
			email += "UNSET"
		}
		identity = name + " <" + email + ">"
	}
	status := previewProcess(cmd, cmd.OutOrStdout(), "commit-lint", "--check-signoff", identity)
	if configured == "" && status == 1 {
		cmd.PrintErrln(
			"set signoff_identity in ~/.config/devtools/<repository>.conf or ~/.config/devtools/devtools.conf",
		)
	}
	return identity, status
}

func runPreview(cmd *cobra.Command) int {
	dir, err := os.MkdirTemp(os.Getenv("TMPDIR"), "commit-preview.")
	if err != nil {
		cmd.PrintErrln(err)
		return 1
	}
	defer os.RemoveAll(dir)
	preview := filepath.Join(dir, "COMMIT_EDITMSG")
	// Keep the copy diagnostic stream and status for missing or unreadable drafts.
	if status := previewProcess(cmd, cmd.OutOrStdout(), "cp", "--", "COMMIT_AGENTMSG", preview); status != 0 {
		return status
	}
	trailers, status := previewOutput(cmd, "git", "interpret-trailers", "--parse", preview)
	if status != 0 {
		return status
	}
	signed := false
	for line := range strings.SplitSeq(trailers, "\n") {
		if strings.HasPrefix(strings.ToLower(line), "signed-off-by:") {
			signed = true
			break
		}
	}
	if !signed {
		identity, identityStatus := previewIdentity(cmd)
		if identityStatus != 0 {
			return identityStatus
		}
		if status = previewProcess(
			cmd,
			cmd.OutOrStdout(),
			"git",
			"-c",
			"trailer.separators=:",
			"interpret-trailers",
			"--in-place",
			"--trailer",
			"Signed-off-by: "+identity,
			preview,
		); status != 0 {
			return status
		}
	}
	return previewProcess(
		cmd,
		cmd.OutOrStdout(),
		"prek",
		"run",
		"--stage",
		"commit-msg",
		"--commit-msg-filename",
		preview,
	)
}
