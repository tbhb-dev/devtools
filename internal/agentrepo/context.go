// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Framing lets callers reject truncated output before publishing any value.
const (
	contextFrame   = "devtools-context-v1\n"
	contextFailure = "devtools-context-error-v1\n"
	contextEnd     = "\ndevtools-context-end"
)

// contextError retains the shell's operator-facing diagnostic verbatim.
type contextError string

func (message contextError) Error() string { return string(message) }

func contextCommand(code *int) *cobra.Command {
	const maxArgs = 2
	var framed bool
	var libDir, shellHome string
	command := &cobra.Command{
		Use:   "context VALUE [ARG]",
		Short: "Discover skill repository, session, and worktree context",
		Long:  "Read root, git-dir, repo-name, state-dir, head, digest FILE, harness, session-id, path PATH, agent-context [ACTION], or ready. Only state-dir creates files. Older binaries without context retain the shell discovery path.",
		Args:  cobra.RangeArgs(1, maxArgs),
		Run: func(cmd *cobra.Command, args []string) {
			argument := ""
			if len(args) == maxArgs {
				argument = args[1]
			}
			home := os.Getenv("HOME")
			if cmd.Flags().Changed("home") {
				home = shellHome
			}
			*code = executeContext(cmd, args[0], argument, libDir, home, framed)
		},
	}
	command.Flags().BoolVar(&framed, "framed", false, "Frame responses for shell callers")
	command.Flags().StringVar(&libDir, "lib-dir", "", "Sourcing skill library directory for harness detection")
	command.Flags().StringVar(&shellHome, "home", "", "Home directory spelling from the sourcing shell")
	return command
}

func executeContext(cmd *cobra.Command, kind, argument, libDir, home string, framed bool) int {
	var diagnostic bytes.Buffer
	originalErr := cmd.ErrOrStderr()
	if framed {
		cmd.SetErr(&diagnostic)
	}
	value, err := contextValue(cmd, kind, argument, libDir, home)
	if err != nil {
		status := 1
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			status = exit.ExitCode()
		} else {
			cmd.PrintErrln(err)
		}
		if framed {
			cmd.Print(contextFailure + diagnostic.String() + contextEnd + "\n")
		}
		return status
	}
	if framed {
		cmd.SetErr(originalErr)
		cmd.PrintErr(diagnostic.String())
		cmd.Print(contextFrame + value + contextEnd + "\n")
	} else if kind != "agent-context" && kind != "ready" {
		cmd.Println(value)
	}
	return 0
}

func contextGit(cmd *cobra.Command, args ...string) (string, error) {
	pinned := [...]string{
		"--no-pager",
		"-c",
		"log.showSignature=false",
		"-c",
		"color.ui=false",
		"-c",
		"color.diff=false",
		"-c",
		"color.status=false",
		"-c",
		"core.quotePath=false",
		"-c",
		"diff.noprefix=false",
		"-c",
		"diff.mnemonicPrefix=false",
		"-c",
		"diff.renames=true",
		"-c",
		"diff.context=3",
	}
	return contextRun(cmd, "git", append(pinned[:], args...)...)
}

func contextRun(cmd *cobra.Command, name string, args ...string) (string, error) {
	process := exec.CommandContext(cmd.Context(), name, args...)
	process.Env = append(os.Environ(), "LC_ALL=C")
	process.Stdin = cmd.InOrStdin()
	process.Stderr = cmd.ErrOrStderr()
	output, err := process.Output()
	if err != nil {
		return "", fmt.Errorf("run context producer: %w", err)
	}
	return strings.TrimRight(string(output), "\n"), nil
}

func contextValue(cmd *cobra.Command, kind, argument, libDir, home string) (string, error) {
	switch kind {
	case "root":
		return contextGit(cmd, "rev-parse", "--show-toplevel")
	case "git-dir":
		return contextGit(cmd, "rev-parse", "--absolute-git-dir")
	case "repo-name":
		common, err := contextGit(cmd, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if err != nil {
			return "", err
		}
		return filepath.Base(filepath.Dir(common)), nil
	case "state-dir":
		dir, err := contextGit(cmd, "rev-parse", "--absolute-git-dir")
		if err != nil {
			return "", err
		}
		dir += "/agent-skills"
		// Match shell directory creation with the caller's permission mask.
		//nolint:gosec // Compatibility with existing shell state directory creation.
		if mkdirErr := os.MkdirAll(dir, 0o777); mkdirErr != nil {
			return "", fmt.Errorf("create state directory: %w", mkdirErr)
		}
		return dir, nil
	case "head":
		return contextHead(cmd)
	case "digest":
		return contextDigest(cmd, argument)
	default:
		return contextOther(cmd, kind, argument, libDir, home)
	}
}

func contextOther(cmd *cobra.Command, kind, argument, libDir, home string) (string, error) {
	switch kind {
	case "harness":
		return contextHarness(libDir), nil
	case "session-id":
		if session := os.Getenv("CLAUDE_CODE_SESSION_ID"); session != "" {
			return session, nil
		}
		return os.Getenv("CODEX_THREAD_ID"), nil
	case "path":
		if strings.HasPrefix(argument, "~/") {
			return home + "/" + argument[2:], nil
		}
		return argument, nil
	case "agent-context":
		return "", contextAgent(cmd, argument)
	case "ready":
		return "", contextReady(cmd)
	default:
		return "", contextError("unknown context value: " + kind)
	}
}

func contextHarness(libDir string) string {
	if harness := os.Getenv("DEVTOOLS_HARNESS"); harness != "" {
		return harness
	}
	if os.Getenv("CLAUDE_CODE_SESSION_ID") != "" || os.Getenv("CLAUDECODE") != "" {
		return "claude"
	}
	if os.Getenv("CODEX_THREAD_ID") != "" {
		return "codex"
	}
	for _, harness := range []struct{ name, path, packaged string }{
		{"claude", "/.claude/skills/", "/packages/agents/claude/"},
		{"codex", "/.agents/skills/", "/packages/agents/codex/"},
	} {
		if strings.Contains(libDir, harness.path) || strings.Contains(libDir, harness.packaged) {
			return harness.name
		}
	}
	return "unknown"
}

func contextAgent(cmd *cobra.Command, action string) error {
	dir, err := contextGit(cmd, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return err
	}
	for _, marker := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply"} {
		if marker == "MERGE_HEAD" && action == "commit" {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(dir, marker)); statErr == nil {
			return contextError("HELD: " + marker + " is in progress; finish or abort it first.")
		}
	}
	if _, symbolicErr := contextGit(cmd, "symbolic-ref", "--quiet", "HEAD"); symbolicErr != nil {
		return contextError("HELD: HEAD is detached.")
	}
	if _, headErr := contextGit(cmd, "rev-parse", "--verify", "--quiet", "HEAD"); headErr != nil {
		return contextError("Create the initial commit through the interactive commit skill first.")
	}
	return nil
}

func contextReady(cmd *cobra.Command) error {
	root, err := contextGit(cmd, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if trustErr := contextTrust(cmd, root); trustErr != nil {
		return trustErr
	}
	if !contextFile(root, ".vale.ini") {
		return nil
	}
	if !contextTool("vale") {
		return nil
	}
	process := exec.CommandContext(cmd.Context(), "vale", "--ext=.md", "--output=JSON")
	process.Env = append(os.Environ(), "LC_ALL=C", "PWD="+root)
	process.Dir = root
	process.Stdin = strings.NewReader("Worktree setup probe.\n")
	output, probeErr := process.CombinedOutput()
	if probeErr == nil {
		return nil
	}
	if exit, ok := errors.AsType[*exec.ExitError](probeErr); ok && exit.ExitCode() == 1 {
		return nil
	}
	message := strings.TrimRight(string(output), "\n")
	if strings.Contains(message, "isn't on the StylesPath") ||
		strings.Contains(message, "StylesPath '") && strings.Contains(message, "doesn't exist") {
		return contextError("worktree setup: Vale styles are missing. Run:\n\n  mise run devtools:worktree-setup")
	}
	return contextError("worktree setup: Vale could not load the repository configuration:\n" + message)
}

func contextTrust(cmd *cobra.Command, root string) error {
	if !contextFile(root, "mise.toml") && !contextFile(root, ".mise.toml") {
		return nil
	}
	if !contextTool("mise") {
		return nil
	}
	process := exec.CommandContext(cmd.Context(), "mise", "-C", root, "trust", "--show")
	process.Env = append(os.Environ(), "LC_ALL=C", "PWD="+root)
	process.Dir = root
	output, err := process.CombinedOutput()
	trust := strings.TrimRight(string(output), "\n")
	if err != nil {
		return contextError("worktree setup: could not check mise trust:\n" + trust)
	}
	if strings.Contains(trust, ": untrusted") {
		return contextError("worktree setup: mise config is untrusted. Run:\n\n  mise trust")
	}
	return nil
}

func contextFile(root, name string) bool {
	info, err := os.Stat(filepath.Join(root, name))
	return err == nil && info.Mode().IsRegular()
}

func contextDigest(cmd *cobra.Command, argument string) (string, error) {
	name, args := "sha256sum", []string{argument}
	if _, err := exec.LookPath(name); err != nil {
		name, args = "shasum", []string{"-a", "256", argument}
	}
	digest, err := contextRun(cmd, name, args...)
	if err != nil {
		return "", err
	}
	value, _, _ := strings.Cut(digest, " ")
	return value, nil
}

func contextTool(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func contextHead(cmd *cobra.Command) (string, error) {
	if _, err := contextGit(cmd, "rev-parse", "--absolute-git-dir"); err != nil {
		return "", err
	}
	ref, err := contextGit(cmd, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 1 {
			return "", err
		}
	}
	if ref != "" {
		_, lookupErr := contextGit(cmd, "show-ref", "--verify", "--quiet", ref)
		if exit, ok := errors.AsType[*exec.ExitError](lookupErr); ok && exit.ExitCode() == 1 {
			return "unborn", nil
		}
		if lookupErr != nil {
			return "", lookupErr
		}
	}
	return contextGit(cmd, "rev-parse", "--verify", "--quiet", "HEAD")
}
