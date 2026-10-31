// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func lintTypesFixture(t *testing.T) (string, string) {
	t.Helper()
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	bash := ""
	if runtime.GOOS != "windows" {
		bash, err = exec.LookPath("bash")
		require.NoError(t, err)
	}
	bin := t.TempDir()
	name := "uv"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	build := exec.CommandContext(t.Context(), "go")
	build.Args = append(build.Args,
		"build",
		"-o",
		filepath.Join(bin, name),
		"./testdata/lint-types/uv.go",
	)
	output, err := build.CombinedOutput()
	require.NoError(t, err, "%s", output)
	if runtime.GOOS != "windows" {
		for _, tool := range []string{"bash", "dirname", "grep"} {
			path, lookErr := exec.LookPath(tool)
			require.NoError(t, lookErr)
			require.NoError(t, os.Symlink(path, filepath.Join(bin, tool)))
		}
	}
	// No agentrepo binary is present: both frozen entry points must still work.
	t.Setenv("PATH", bin)
	t.Setenv("DEVTOOLS_DIR", filepath.Join(root, ".devtools"))
	t.Setenv("LC_ALL", "POSIX")
	t.Setenv("LINT_TYPES_UNRELATED", "unchanged")
	return root, bash
}

func TestLintTypes(t *testing.T) {
	root, bash := lintTypesFixture(t)
	const emptyView = "No Python files matched patterns"
	const skip = "lint-types: pyrefly matched no files behind the worktree's hidden path; skipping — CI checks the real tree\n"
	cases := []struct {
		name, directory, stdout, stderr string
		status                          int
		want                            string
		wantStatus                      int
	}{
		{"empty", "ordinary", "", "", 0, "\n", 0},
		{"stdout", "ordinary", "checked", "", 0, "checked\n", 0},
		{"stderr", "ordinary", "", "checked\n\n\n", 0, "checked\n", 0},
		{"mixed", "ordinary", "one\n", "two\n", 0, "one\ntwo\none\n", 0},
		{"newlines", "ordinary", "ok\n\n\n", "", 0, "ok\n", 0},
		{"hidden-out", ".claude/worktrees/one", emptyView, "", 1, skip, 0},
		{"hidden-err", ".claude/worktrees/two", "", emptyView, 2, skip, 0},
		{"embedded", ".claude/worktrees/three", "prefix " + emptyView + " suffix", "", 42, skip, 0},
		{"success-first", ".claude/worktrees/four", emptyView, "", 0, emptyView + "\n", 0},
		{"ordinary-empty-view", "ordinary", emptyView, "", 1, emptyView + "\n", 1},
		{"unrelated", ".claude/worktrees/five", "other\n\n", "", 23, "other\n", 23},
		{"near-match", ".claude/worktrees-other/six", emptyView, "", 7, emptyView + "\n", 7},
		{"no-child", ".claude/worktrees", emptyView, "", 3, emptyView + "\n", 3},
		{"empty-failure", "ordinary", "", "", 127, "\n", 127},
		{"mixed-failure", "ordinary", "one\n", "two\n", 255, "one\ntwo\none\n", 255},
		{"logical-hidden", ".claude/worktrees/link", emptyView, "", 1, skip, 0},
		{"logical-ordinary", "link", emptyView, "", 1, emptyView + "\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && tc.directory != "ordinary" {
				t.Skip("Unix directory spelling; ordinary paths run on every platform")
			}
			lintTypesDirectory(t, tc.name, tc.directory)
			trace := "lint-types-trace"
			t.Setenv("LINT_TYPES_STATUS", strconv.Itoa(tc.status))
			chunks := []map[string]string{{"Stream": "out", "Text": tc.stdout}, {"Stream": "err", "Text": tc.stderr}}
			if strings.HasPrefix(tc.name, "mixed") {
				chunks = append(chunks, chunks[0])
			}
			data, err := json.Marshal(chunks)
			require.NoError(t, err)
			t.Setenv("LINT_TYPES_CHUNKS", string(data))
			beforeEnv := os.Environ()
			beforeDir, err := os.Getwd()
			require.NoError(t, err)
			args := []string{"lint-types", "--unknown", "positional", "--help"}
			var out, stderr bytes.Buffer
			code := agentrepo.Run(args, nil, &out, &stderr)
			require.Equal(t, tc.wantStatus, code, stderr.String())
			require.Equal(t, tc.want, out.String())
			require.Empty(t, stderr.String())
			afterDir, err := os.Getwd()
			require.NoError(t, err)
			require.Equal(t, beforeDir, afterDir)
			require.True(t, slices.Equal(beforeEnv, os.Environ()), "parent environment changed")
			lintTypesTrace(t, trace, beforeDir)
			if runtime.GOOS != "windows" {
				require.NotEmpty(t, bash)
				for _, entry := range []string{".devtools/scripts/lint-types.sh", "tools/lint-types.sh"} {
					require.NoError(t, os.Remove(trace))
					oracle := exec.CommandContext(t.Context(), bash)
					oracle.Args = append(oracle.Args, filepath.Join(root, entry))
					oracle.Args = append(oracle.Args, args[1:]...)
					var oldOut, oldErr bytes.Buffer
					oracle.Stdout, oracle.Stderr = &oldOut, &oldErr
					require.Equal(t, code, lintTypesExit(t, oracle.Run()))
					require.Equal(t, out.String(), oldOut.String())
					require.Equal(t, stderr.String(), oldErr.String())
					lintTypesTrace(t, trace, beforeDir)
				}
			}
		})
	}
}

func lintTypesDirectory(t *testing.T, name, directory string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), filepath.FromSlash(directory))
	require.NoError(t, os.MkdirAll(filepath.Dir(dir), 0o700))
	if strings.HasPrefix(name, "logical-") {
		target := t.TempDir()
		if name == "logical-ordinary" {
			target = filepath.Join(target, ".claude", "worktrees", "physical")
			require.NoError(t, os.MkdirAll(target, 0o700))
		}
		require.NoError(t, os.Symlink(target, dir))
	} else {
		require.NoError(t, os.MkdirAll(dir, 0o700))
	}
	t.Chdir(dir)
	t.Setenv("PWD", dir)
}

func lintTypesTrace(t *testing.T, path, directory string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var calls []json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	for decoder.More() {
		var call json.RawMessage
		require.NoError(t, decoder.Decode(&call))
		calls = append(calls, call)
	}
	require.Len(t, calls, 1)
	var call struct {
		Args      []string `json:"Args"`
		Locale    string   `json:"Locale"`
		Directory string   `json:"Directory"`
		Unrelated string   `json:"Unrelated"`
	}
	require.NoError(t, json.Unmarshal(calls[0], &call))
	require.Equal(t, []string{"run", "pyrefly", "check"}, call.Args)
	require.Equal(t, "C", call.Locale)
	require.Equal(t, directory, call.Directory)
	require.Equal(t, "unchanged", call.Unrelated)
}

func lintTypesExit(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	return exit.ExitCode()
}

func TestLintTypesMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Frozen Bash command-not-found diagnostic is Unix only")
	}
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	bash, err := exec.LookPath("bash")
	require.NoError(t, err)
	t.Chdir(root)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("BASH_ENV", "")
	var out, stderr, oldOut, oldErr bytes.Buffer
	code := agentrepo.Run([]string{"lint-types"}, nil, &out, &stderr)
	oracle := exec.CommandContext(t.Context(), bash, ".devtools/scripts/lint-types.sh")
	oracle.Stdout, oracle.Stderr = &oldOut, &oldErr
	require.Equal(t, 127, lintTypesExit(t, oracle.Run()))
	require.Equal(t, 127, code)
	require.Equal(t, oldOut.String(), out.String())
	require.Contains(t, out.String(), "uv: command not found\n")
	require.Empty(t, stderr.String())
	require.Empty(t, oldErr.String())
}

func TestLintTypesHelp(t *testing.T) {
	t.Parallel()
	var out, stderr bytes.Buffer
	require.Equal(t, 0, agentrepo.Run([]string{"help", "lint-types"}, nil, &out, &stderr))
	require.Contains(t, out.String(), "uv run pyrefly check")
	require.Empty(t, stderr.String())
}
