// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

func lintTypesCommand(code *int) *cobra.Command {
	return &cobra.Command{
		Use:   "lint-types",
		Short: "Run uv run pyrefly check with the existing worktree exemption",
		Long: "Run uv run pyrefly check with LC_ALL=C. Extra arguments are ignored. " +
			"PATH must be non-empty. Relative entries and empty entries inside it are supported. " +
			"This additive command leaves consumer tasks on the existing shell adapter. " +
			"Use agentrepo help lint-types to display this help.",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, _ []string) {
			path, executable := lintTypesPath()
			if !executable {
				message := "uv: command not found"
				*code = 127
				if path != "" {
					message = path + ": Permission denied"
					*code = 126
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), ".devtools/scripts/lint-types.sh: line 23: "+message)
				return
			}
			child := exec.CommandContext(cmd.Context(), path, "run", "pyrefly", "check")
			child.Env = append(os.Environ(), "LC_ALL=C")
			child.Stdin = cmd.InOrStdin()
			output, err := child.CombinedOutput()
			status := 0
			if err != nil {
				status = fuzzExit(err, cmd.OutOrStdout())
			}
			// The current directory retains valid logical spelling through symlinks, like Bash.
			directory, cwdErr := os.Getwd()
			if status != 0 && cwdErr == nil && strings.Contains(directory, "/.claude/worktrees/") &&
				strings.Contains(string(output), "No Python files matched patterns") {
				_, _ = fmt.Fprintln(
					cmd.OutOrStdout(),
					"lint-types: pyrefly matched no files behind the worktree's hidden path; skipping — CI checks the real tree",
				)
				return
			}
			// Command substitution removes every trailing newline. Printing adds exactly one.
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), strings.TrimRight(string(output), "\n"))
			*code = status
		},
	}
}

// lintTypesRelativeEntry accepts a lookup result Go flagged as relative only when
// a relative PATH entry produced it. Without such an entry, a binary in the
// current directory goes unmatched. The returned path contains a separator, which
// makes the later run skip a second lookup.
func lintTypesRelativeEntry(path, pathEnv string) (string, bool) {
	clean := filepath.Clean(path)
	for _, directory := range filepath.SplitList(pathEnv) {
		if directory == "" || filepath.IsAbs(directory) {
			continue
		}
		if filepath.Join(directory, filepath.Base(clean)) == clean {
			return "." + string(filepath.Separator) + clean, true
		}
	}
	return "", false
}

// lintTypesExecutable reports whether the caller may execute the file. The Unix build
// replaces it with a kernel access check, and Windows never reaches it.
//
//nolint:gochecknoglobals // Platform initialization selects the implementation before use.
var lintTypesExecutable = func(string) bool { return false }

func lintTypesPath() (string, bool) {
	if runtime.GOOS == "windows" {
		path, err := exec.LookPath("uv")
		if errors.Is(err, exec.ErrDot) {
			return lintTypesRelativeEntry(path, os.Getenv("PATH"))
		}
		return path, err == nil
	}
	denied := ""
	for directory := range strings.SplitSeq(os.Getenv("PATH"), ":") {
		if directory == "" {
			directory = "."
		}
		candidate := directory + "/uv"
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			if lintTypesExecutable(candidate) {
				return candidate, true
			}
			if denied == "" {
				denied = candidate
			}
		}
	}
	return denied, false
}
