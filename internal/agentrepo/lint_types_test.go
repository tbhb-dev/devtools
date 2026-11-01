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

func lintTypesFixture(t *testing.T) (string, string, string) {
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
	return root, bash, bin
}

func TestLintTypes(t *testing.T) {
	root, bash, _ := lintTypesFixture(t)
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
		{"relative-path", "ordinary", "checked\n", "failed\n", 23, "checked\nfailed\n", 23},
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
			lintTypesTrace(t, beforeDir)
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
					t.Logf("%s: status=%d stdout=%q stderr=%q", entry, code, oldOut.String(), oldErr.String())
					require.Equal(t, out.String(), oldOut.String())
					require.Equal(t, stderr.String(), oldErr.String())
					lintTypesTrace(t, beforeDir)
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
	if name == "relative-path" {
		bin, err := filepath.Rel(dir, os.Getenv("PATH"))
		require.NoError(t, err)
		t.Setenv("PATH", bin)
	}
}

func lintTypesTrace(t *testing.T, directory string) {
	t.Helper()
	data, err := os.ReadFile("lint-types-trace")
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

func TestLintTypesLookup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Frozen Bash command lookup is Unix only")
	}
	root, bash, bin := lintTypesFixture(t)
	binary := filepath.Join(bin, "uv")
	lintTypesLogOracle(t, bash)
	t.Setenv("BASH_ENV", "")
	t.Setenv("LINT_TYPES_CHUNKS", `[{"Stream":"out","Text":"checked"}]`)
	for _, path := range []string{".", "./bin", "absolute", ":./bin", "./bin:", ".:./bin", "./bin:."} {
		for _, tc := range []struct {
			state  string
			status int
		}{
			{"success", 0},
			{"failure", 23},
			{"denied", 0},
			{"missing", 0},
			{"absolute-link", 0},
			{"relative-link", 0},
			{"failure-link", 23},
			{"denied-link", 0},
			{"other-execute", 0},
			{"other-execute-first", 0},
		} {
			state, status := tc.state, tc.status
			t.Run(path+"/"+state, func(t *testing.T) {
				lintTypesSkipRoot(t, state)
				dir := t.TempDir()
				t.Chdir(dir)
				t.Setenv("PWD", dir)
				require.NoError(t, os.Symlink(filepath.Join(root, ".devtools"), ".devtools"))
				lintTypesLookupPath(t, path, state, binary)
				t.Setenv("LINT_TYPES_STATUS", strconv.Itoa(status))
				var out, stderr, oldOut, oldErr bytes.Buffer
				oracle := exec.CommandContext(t.Context(), bash, ".devtools/scripts/lint-types.sh")
				oracle.Stdout, oracle.Stderr = &oldOut, &oldErr
				wantCode := lintTypesExit(t, oracle.Run())
				t.Logf("PATH=%q state=%s: status=%d stdout=%q stderr=%q",
					path, state, wantCode, oldOut.String(), oldErr.String())
				if wantCode == status {
					lintTypesTrace(t, dir)
					require.NoError(t, os.Remove("lint-types-trace"))
				}
				code := agentrepo.Run([]string{"lint-types"}, nil, &out, &stderr)
				require.Equal(t, wantCode, code)
				require.Equal(t, oldOut.String(), out.String())
				require.Equal(t, oldErr.String(), stderr.String())
				if wantCode == status {
					lintTypesTrace(t, dir)
				}
			})
		}
	}
}

// lintTypesLogOracle records which Bash judges the lookup cases and the PATH
// it falls back to when none is set. A CI log then shows both.
func lintTypesLogOracle(t *testing.T, bash string) {
	t.Helper()
	version, err := exec.CommandContext(t.Context(), bash, "--version").Output()
	require.NoError(t, err)
	first, _, _ := strings.Cut(string(version), "\n")
	probe := exec.CommandContext(t.Context(), bash, "-c", `printf %s "$PATH"`)
	probe.Env = []string{}
	fallback, err := probe.Output()
	require.NoError(t, err)
	t.Logf("oracle %s: %s; PATH when unset: %q", bash, first, fallback)
}

func lintTypesLookupPath(t *testing.T, path, state, binary string) {
	t.Helper()
	bin := "."
	if strings.Contains(path, "bin") {
		bin = "./bin"
	}
	if path == "absolute" {
		bin = t.TempDir()
		path = bin
	}
	require.NoError(t, os.MkdirAll(bin, 0o700))
	lintTypesLookupFile(t, bin, state, binary)
	if path == ".:./bin" || path == "./bin:." {
		require.NoError(t, os.WriteFile("uv", nil, 0o600))
		if state == "other-execute-first" {
			lintTypesOtherExecute(t, "uv", binary)
		}
	}
	t.Setenv("PATH", path)
}

func lintTypesLookupFile(t *testing.T, bin, state, binary string) {
	t.Helper()
	if state == "missing" {
		return
	}
	uv := bin + "/uv"
	target := uv
	if strings.Contains(state, "link") {
		target = filepath.Join(t.TempDir(), "target")
	}
	switch {
	case state == "other-execute":
		lintTypesOtherExecute(t, target, binary)
	case strings.HasPrefix(state, "denied"):
		require.NoError(t, os.WriteFile(target, nil, 0o600))
	default:
		require.NoError(t, os.Link(binary, target))
	}
	if state == "relative-link" {
		dir, err := os.Getwd()
		require.NoError(t, err)
		target, err = filepath.Rel(filepath.Join(dir, bin), target)
		require.NoError(t, err)
	}
	if strings.Contains(state, "link") {
		require.NoError(t, os.Symlink(target, uv))
	}
}

// lintTypesOtherExecute copies the stub with mode 0641, so the file's own user is denied execution.
func lintTypesOtherExecute(t *testing.T, path, binary string) {
	t.Helper()
	content, err := os.ReadFile(binary)
	require.NoError(t, err)
	//nolint:gosec // The path sits in a private temporary directory.
	require.NoError(t, os.WriteFile(path, content, 0o600))
	//nolint:gosec // The fixture must deny execution to its own user.
	require.NoError(t, os.Chmod(path, 0o641))
}

func lintTypesSkipRoot(t *testing.T, state string) {
	t.Helper()
	if strings.HasPrefix(state, "other-execute") && os.Geteuid() == 0 {
		t.Skip("Root can execute a file that any class may execute")
	}
}

func TestLintTypesHelp(t *testing.T) {
	t.Parallel()
	var out, stderr bytes.Buffer
	require.Equal(t, 0, agentrepo.Run([]string{"help", "lint-types"}, nil, &out, &stderr))
	require.Contains(t, out.String(), "uv run pyrefly check")
	require.Empty(t, stderr.String())
}
