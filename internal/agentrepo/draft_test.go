// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

const controlText = "This is a very robust and comprehensive design that does not use contractions and it is significantly better.\n"

type draftFixture struct {
	root, bin, binary, legacy, shim, outside, log string
}

type draftResult struct {
	out, stderr string
	code        int
}

type draftCall struct {
	Name   string   `json:"Name"`
	Input  string   `json:"Input"`
	Cwd    string   `json:"Cwd"`
	Locale string   `json:"Locale"`
	UTF8   string   `json:"UTF8"`
	Args   []string `json:"Args"`
}

func draftBuild(tb testing.TB, output, source string) {
	tb.Helper()
	cmd := exec.CommandContext(tb.Context(), "go", "build", "-o", output, source)
	data, err := cmd.CombinedOutput()
	require.NoError(tb, err, "%s", data)
}

func newDraftFixture(tb testing.TB, cli bool) draftFixture {
	tb.Helper()
	repo, err := filepath.Abs("../..")
	require.NoError(tb, err)
	base := tb.TempDir()
	f := draftFixture{
		root: filepath.Join(base, "repo"), bin: filepath.Join(base, "bin"),
		binary:  filepath.Join(base, "agentrepo"),
		legacy:  filepath.Join(repo, "internal/agentrepo/testdata/draft/legacy.sh"),
		shim:    filepath.Join(repo, ".devtools/scripts/lint-draft.sh"),
		outside: filepath.Join(base, "outside.md"), log: filepath.Join(base, "calls"),
	}
	require.NoError(tb, os.Mkdir(f.root, 0o700))
	require.NoError(tb, os.Mkdir(f.bin, 0o700))
	draftBuild(tb, filepath.Join(f.bin, "gate"), "testdata/draft/gates.go")
	if cli {
		draftBuild(tb, f.binary, "../../cmd/agentrepo")
		require.NoError(tb, os.Symlink(f.binary, filepath.Join(f.bin, "agentrepo")))
	}
	for _, gate := range []string{"vale", "cspell", "rumdl"} {
		require.NoError(tb, os.Symlink("gate", filepath.Join(f.bin, gate)))
	}
	tb.Setenv("PATH", f.bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	tb.Setenv("DRAFT_LOG", f.log)
	tb.Setenv("DRAFT_PROBE_OUT", "1:1 [warning] scope  finding\n")
	tb.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	tb.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	for _, key := range []string{"GIT_DIR", "GIT_COMMON_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"} {
		tb.Setenv(key, "")
		require.NoError(tb, os.Unsetenv(key))
	}
	cmd := exec.CommandContext(tb.Context(), "git", "init", "--quiet")
	cmd.Dir = f.root
	data, err := cmd.CombinedOutput()
	require.NoError(tb, err, "%s", data)
	for _, name := range []string{"draft.md", "draft.txt", "draft.go", "draft.py", "draft.sh", "--draft.md"} {
		require.NoError(tb, os.WriteFile(filepath.Join(f.root, name), []byte("# Draft\n\nA draft.\n"), 0o600))
	}
	require.NoError(tb, os.WriteFile(f.outside, []byte("Outside.\n"), 0o600))
	require.NoError(tb, os.Symlink(f.outside, filepath.Join(f.root, "linked.md")))
	require.NoError(tb, os.MkdirAll(filepath.Join(base, "outside", "nested"), 0o700))
	require.NoError(tb, os.WriteFile(filepath.Join(base, "outside", "draft.md"), []byte("Outside.\n"), 0o600))
	require.NoError(tb, os.Symlink(filepath.Join(base, "outside", "nested"), filepath.Join(f.root, "escape")))
	tb.Chdir(f.root)
	return f
}

func draftRun(args ...string) draftResult {
	var out, stderr bytes.Buffer
	code := agentrepo.Run(append([]string{"lint-draft"}, args...), nil, &out, &stderr)
	return draftResult{out.String(), stderr.String(), code}
}

func (f draftFixture) calls(tb testing.TB) []draftCall {
	tb.Helper()
	data, err := os.ReadFile(f.log)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(tb, err)
	var calls []draftCall
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var call draftCall
		require.NoError(tb, json.Unmarshal([]byte(line), &call))
		calls = append(calls, call)
	}
	return calls
}

func (f draftFixture) clearCalls(tb testing.TB) {
	tb.Helper()
	err := os.Remove(f.log)
	if !os.IsNotExist(err) {
		require.NoError(tb, err)
	}
}

func TestLintDraftRejected(t *testing.T) {
	t.Setenv("LC_ALL", "C.UTF-8")
	f := newDraftFixture(t, false)
	for _, tc := range []struct {
		file, finding string
		code          int
	}{
		{"", "", 2},
		{"absent.md", "missing-file  no such file", 1},
		{".", "missing-file  no such file", 1},
		{f.outside, "outside-repo", 1},
		{"../outside.md", "outside-repo", 1},
		{"linked.md", "outside-repo", 1},
		{"escape/../draft.md", "outside-repo", 1},
		{"draft.go", "wrong-task", 2},
		{"draft.py", "wrong-task", 2},
		{"draft.sh", "wrong-task", 2},
	} {
		t.Run(tc.file, func(t *testing.T) {
			t.Setenv("LC_ALL", "C.UTF-8")
			f.clearCalls(t)
			got := draftRun(tc.file)
			require.Equal(t, tc.code, got.code)
			if tc.file == "" {
				require.Equal(t, "usage: lint-draft.sh <file>\n", got.stderr)
				require.Empty(t, got.out)
			} else {
				require.Contains(t, got.out, tc.file+":1 [error] "+tc.finding)
				require.Empty(t, got.stderr)
			}
			require.Empty(t, f.calls(t))
		})
	}
	require.Equal(t, draftResult{stderr: "usage: lint-draft.sh <file>\n", code: 2}, draftRun())
}

func TestLintDraftGates(t *testing.T) {
	t.Setenv("LC_ALL", "C.UTF-8")
	f := newDraftFixture(t, false)
	for _, file := range []string{"draft.md", "draft.txt"} {
		for failures := range 8 {
			t.Run(fmt.Sprintf("%s/%d", file, failures), func(t *testing.T) {
				t.Setenv("LC_ALL", "C.UTF-8")
				assertDraftGates(t, f, file, failures)
			})
		}
	}
}

func assertDraftGates(t *testing.T, f draftFixture, file string, failures int) {
	t.Helper()
	f.clearCalls(t)
	var wantOut, wantErr strings.Builder
	var names []string
	for bit, gate := range []string{"vale", "cspell", "rumdl"} {
		failed := failures&(1<<bit) != 0
		key := "DRAFT_" + strings.ToUpper(gate)
		t.Setenv(key+"_CODE", strconv.Itoa(failures&(1<<bit)))
		stdout, stderr := "", ""
		if failed {
			stdout, stderr = gate+" finding\n", gate+" diagnostic\n"
		}
		t.Setenv(key+"_OUT", stdout)
		t.Setenv(key+"_ERR", stderr)
		if gate != "rumdl" || strings.HasSuffix(file, ".md") {
			names = append(names, gate)
			wantOut.WriteString(stdout)
			wantErr.WriteString(stderr)
		}
	}
	wantCode := 0
	if wantOut.Len() != 0 {
		wantCode = 1
	}
	require.Equal(t, draftResult{wantOut.String(), wantErr.String(), wantCode}, draftRun(file))
	gotNames := make([]string, 0, len(names)+1)
	for _, call := range f.calls(t) {
		gotNames = append(gotNames, call.Name)
	}
	require.Equal(t, append([]string{"probe"}, names...), gotNames)
}

func TestLintDraftProbe(t *testing.T) {
	t.Setenv("LC_ALL", "C.UTF-8")
	f := newDraftFixture(t, false)
	for _, tc := range []struct {
		name, stdout, stderr, finding string
		code                          int
	}{
		{"clean", "1:1 scoped\n", "", "", 0},
		{"finding on stderr", "", "2:1 scoped\n", "", 1},
		{"unscoped zero", "", "", "unscoped-path", 0},
		{"unscoped one", "noise\n", "", "unscoped-path", 1},
		{"failed", "", "failure\ndetail\n\n", "vale-failed  vale exited 2 instead of linting, so this run measured nothing: failure detail", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.clearCalls(t)
			t.Setenv("DRAFT_PROBE_OUT", tc.stdout)
			t.Setenv("DRAFT_PROBE_ERR", tc.stderr)
			t.Setenv("DRAFT_PROBE_CODE", strconv.Itoa(tc.code))
			path := filepath.Join(f.root, "draft.md")
			before, err := os.Stat(path)
			require.NoError(t, err)
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			got := draftRun("draft.md")
			if tc.finding == "" {
				require.Equal(t, draftResult{}, got)
			} else {
				require.Equal(t, 1, got.code)
				require.Contains(t, got.out, "draft.md:1 [error] "+tc.finding)
				require.Empty(t, got.stderr)
			}
			if tc.code > 1 {
				require.NotContains(t, got.out, "unscoped-path")
			}
			calls := f.calls(t)
			require.Len(t, calls, 4)
			require.Equal(t, draftCall{
				Name: "probe", Input: controlText, Cwd: f.root, Locale: "C", UTF8: "1",
				Args: []string{"--path=draft.md", "--output=ai-tells-agent.tmpl"},
			}, calls[0])
			require.Equal(t, []string{"--output=ai-tells-agent.tmpl", "draft.md"}, calls[1].Args)
			require.Equal(
				t,
				[]string{"--config", ".cspell.jsonc", "--no-summary", "--no-progress", "draft.md"},
				calls[2].Args,
			)
			require.Equal(t, []string{"check", "draft.md"}, calls[3].Args)
			after, err := os.Stat(path)
			require.NoError(t, err)
			require.Equal(t, before.ModTime(), after.ModTime())
			afterContent, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, content, afterContent)
		})
	}
}

func TestLintDraftStartFailure(t *testing.T) {
	t.Setenv("LC_ALL", "C.UTF-8")
	f := newDraftFixture(t, false)
	for _, gate := range []string{"cspell", "vale"} {
		t.Run(gate, func(t *testing.T) {
			f.clearCalls(t)
			path := filepath.Join(f.bin, gate)
			require.NoError(t, os.Remove(path))
			// An existing non-executable gate prevents fallback to an installed gate.
			require.NoError(t, os.WriteFile(path, nil, 0o600))
			t.Setenv("PATH", f.bin+":/usr/bin:/bin")
			got := draftRun("draft.md")
			require.Equal(t, 1, got.code)
			require.Contains(t, got.stderr, gate)
			if gate == "vale" {
				require.Contains(t, got.out, "vale-failed")
				require.NotContains(t, got.out, "unscoped-path")
			}
			calls := f.calls(t)
			require.Equal(t, "rumdl", calls[len(calls)-1].Name)
			require.NoError(t, os.Remove(path))
			require.NoError(t, os.Symlink("gate", path))
		})
	}
}

func TestLintDraftOutsideGit(t *testing.T) {
	t.Setenv("LC_ALL", "C.UTF-8")
	t.Chdir(t.TempDir())
	got := draftRun("draft.md")
	require.Equal(t, 1, got.code)
	require.Empty(t, got.out)
	require.Contains(t, got.stderr, "not a git repository")
}

func TestLintDraftRootRelative(t *testing.T) {
	t.Setenv("LC_ALL", "C.UTF-8")
	f := newDraftFixture(t, false)
	subdir := filepath.Join(f.root, "sub")
	require.NoError(t, os.Mkdir(subdir, 0o700))
	t.Chdir(subdir)
	require.Equal(t, draftResult{}, draftRun("draft.md"))
	require.Equal(t, draftResult{}, draftRun(filepath.Join(f.root, "draft.md")))
}

const draftBudget = 200 * time.Millisecond
