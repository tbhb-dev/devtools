// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//nolint:paralleltest // Tests change the process working directory and environment.
package tokens_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/tokens"
)

func parityBackends(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	binary := filepath.Join(bin, "uv")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "testdata/parity_backend.go")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	for _, name := range []string{"ant", "agentrepo", "git"} {
		require.NoError(t, os.Link(binary, filepath.Join(bin, name)))
	}
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	t.Setenv("TOKEN_REAL_GIT", realGit)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, key := range []string{"MODEL", "CLAUDE_MODEL", "CODEX_MODEL", "CODEX_ENCODING", "BODY_BUDGET", "TOKEN_FAIL", "TOKEN_FAIL_AT"} {
		t.Setenv(key, "")
	}
}

func parityWrite(t *testing.T, root, path, text string) {
	t.Helper()
	path = filepath.Join(root, filepath.FromSlash(path))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
}

func parityGit(t *testing.T, root string, args ...string) {
	t.Helper()
	//nolint:gosec // G204: the executable is the real Git found on PATH and the arguments are fixed by the tests.
	cmd := exec.CommandContext(t.Context(), os.Getenv("TOKEN_REAL_GIT"), args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
}

// runFailureBackend runs one backend and reports false when the frozen shell isn't exercised for the stage.
func runFailureBackend(t *testing.T, backend, shell, selected, mode, stage string) bool {
	t.Helper()
	var out, stderr bytes.Buffer
	if backend == "go" {
		require.NotZero(t, tokens.Run(t.Context(), []string{selected}, &out, &stderr), stderr.String())
		return true
	}
	// The shell's wc/awk failures are covered by its frozen Bats suite;
	// Go measures bytes and totals directly, exercised by its unit tests.
	if stage == "measurement" || stage == "total" || (stage == "discovery" && mode == "invalid") {
		return false
	}
	cmd := exec.CommandContext(t.Context(), "bash", shell, selected)
	cmd.Stdout, cmd.Stderr = &out, &stderr
	require.Error(t, cmd.Run())
	return true
}

func writeFailureFixture(t *testing.T, root string) (string, string) {
	t.Helper()
	dir := "packages/agents/codex/.apm/skills/first"
	second := "packages/agents/codex/.apm/skills/second"
	for _, d := range []string{dir, second, "claude"} {
		parityWrite(t, root, d+"/SKILL.md", "body\n")
		parityWrite(t, root, d+"/extra", "bundled content\n")
		parityWrite(t, root, d+"/extra2", "another bundle\n")
	}
	parityWrite(t, root, second+"/SKILL.md", "second body\n")
	return dir, second
}

// assertFailureKeepsRecord checks that a failure at stage leaves the previous token record untouched.
func assertFailureKeepsRecord(t *testing.T, root, shell, selected, mode, stage string) {
	t.Helper()
	t.Setenv("TOKEN_FAIL", mode)
	t.Setenv("TOKEN_FAIL_AT", stage)
	for _, backend := range []string{"shell", "go"} {
		parityWrite(t, root, selected+"/tokens.json", "preserved")
		if !runFailureBackend(t, backend, shell, selected, mode, stage) {
			continue
		}
		record, err := os.ReadFile(filepath.Join(root, selected, "tokens.json"))
		require.NoError(t, err)
		require.Equal(t, "preserved", string(record))
	}
}

func TestFrozenShellFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("frozen shell comparison requires Bash")
	}
	shell, err := filepath.Abs("../../tools/skill-tokens.sh")
	require.NoError(t, err)
	parityBackends(t)
	root := t.TempDir()
	parityGit(t, root, "init", "--quiet")
	t.Chdir(root)
	dir, second := writeFailureFixture(t, root)
	for _, mode := range []string{"silent", "partial", "invalid"} {
		for _, stage := range []string{"count", "bundled", "discovery", "measurement", "total", "claude"} {
			selected := dir
			if stage == "claude" {
				selected = "claude"
			}
			t.Run(mode+"/"+stage, func(t *testing.T) {
				assertFailureKeepsRecord(t, root, shell, selected, mode, stage)
			})
		}
	}
	t.Setenv("TOKEN_FAIL", "partial")
	t.Setenv("TOKEN_FAIL_AT", "second")
	var out, stderr bytes.Buffer
	require.NotZero(t, tokens.Run(t.Context(), []string{dir, second}, &out, &stderr))
	record, err := os.ReadFile(filepath.Join(root, dir, "tokens.json"))
	require.NoError(t, err)
	require.Contains(t, string(record), `"skill": "first"`)
}

// runParityBackend runs the frozen shell or the Go command and returns its stdout and stderr.
func runParityBackend(t *testing.T, backend, shell string, args []string) (string, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	if backend != "shell" {
		require.Zero(t, tokens.Run(t.Context(), args, &out, &stderr), stderr.String())
		return out.String(), stderr.String()
	}
	//nolint:gosec // G204: the fixed bash executable receives the repository's frozen script and test-chosen arguments.
	cmd := exec.CommandContext(t.Context(), "bash", append([]string{shell}, args...)...)
	cmd.Stdout, cmd.Stderr = &out, &stderr
	require.NoError(t, cmd.Run(), stderr.String())
	return out.String(), stderr.String()
}

type parityCase struct {
	name string
	args []string
	env  map[string]string
}

func writeParityFixture(t *testing.T, root string) []string {
	t.Helper()
	dirs := []string{
		"packages/agents/codex/.apm/skills/local",
		"packages/agents/claude/.apm/skills/remote",
		"packages/agents/claude/.apm/skills/plain",
		".agents/skills/deployed",
		"custom",
	}
	files := map[string]string{
		"untracked.txt": "untracked\n",
		"empty":         "",
		"binary":        "a\x00b",
		"ignored":       "ignored",
		"é😀":            "UTF-8\r\n",
		"tokens.json":   "old",
	}
	for _, dir := range dirs {
		parityWrite(
			t,
			root,
			dir+"/SKILL.md",
			"---\nname: example\ndescription: >-\n  Fold café\n  and words.\n---\nBody café.\n",
		)
		parityWrite(t, root, dir+"/tracked.txt", "tracked café\n")
		parityGit(t, root, "add", "--", dir+"/SKILL.md", dir+"/tracked.txt")
		for name, text := range files {
			parityWrite(t, root, dir+"/"+name, text)
		}
	}
	parityWrite(t, root, dirs[2]+"/SKILL.md", "plain\n")
	parityWrite(t, root, "custom/SKILL.md", "")
	parityWrite(t, root, ".gitignore", "ignored\n")
	return dirs
}

func parityCases(root string, dirs []string) []parityCase {
	overrides := map[string]string{
		"MODEL":          "generic",
		"CLAUDE_MODEL":   "explicit",
		"CODEX_MODEL":    "local-model",
		"CODEX_ENCODING": "local-encoding",
	}
	return []parityCase{
		{"default", nil, nil},
		{"explicit", []string{dirs[0] + "/", dirs[1], dirs[2], dirs[3], "custom", "missing/"}, nil},
		{
			"absolute",
			[]string{filepath.Join(root, dirs[0]), filepath.Join(root, dirs[1]), filepath.Join(root, dirs[3])},
			nil,
		},
		{"model", dirs, map[string]string{"MODEL": "generic"}},
		{"overrides", dirs, overrides},
		{"claude-only", []string{dirs[1], dirs[2]}, map[string]string{"CLAUDE_MODEL": "explicit"}},
		{"budget-13", []string{dirs[0]}, map[string]string{"BODY_BUDGET": "13"}},
		{"budget-12", []string{dirs[0]}, map[string]string{"BODY_BUDGET": "12"}},
		{"budget-11", []string{dirs[0]}, map[string]string{"BODY_BUDGET": "11"}},
	}
}

// assertParityCase runs the shell, Go and a repeated Go invocation and compares every observable.
func assertParityCase(t *testing.T, root, shell, log string, dirs []string, tc parityCase) {
	t.Helper()
	for key, value := range tc.env {
		t.Setenv(key, value)
	}
	var wantRecords [][]byte
	var wantOut, wantErr, wantLog string
	for _, backend := range []string{"shell", "go", "repeat"} {
		require.NoError(t, os.WriteFile(log, nil, 0o600))
		out, stderr := runParityBackend(t, backend, shell, tc.args)
		records := make([][]byte, len(dirs))
		for i, dir := range dirs {
			record, err := os.ReadFile(filepath.Join(root, dir, "tokens.json"))
			require.NoError(t, err)
			records[i] = record
		}
		requests, err := os.ReadFile(log)
		require.NoError(t, err)
		if backend == "shell" {
			wantRecords, wantOut, wantErr, wantLog = records, out, stderr, string(requests)
			continue
		}
		require.Equal(t, wantRecords, records, "record bytes")
		if strings.HasPrefix(tc.name, "budget-") {
			require.NotContains(t, string(requests), "ant ", "Codex stays local")
		} else {
			require.Equal(t, 1, strings.Count(string(requests), `content: |\n      x\n`), "one Claude baseline")
		}
		require.Equal(t, wantOut, out, "report")
		require.Equal(t, wantErr, stderr, "skip diagnostics")
		require.Equal(t, wantLog, string(requests), "backend, model, encoding, payload and one envelope")
	}
}

func TestFrozenShellParity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("frozen shell comparison requires Bash")
	}
	shell, err := filepath.Abs("../../tools/skill-tokens.sh")
	require.NoError(t, err)
	parityBackends(t)
	root := t.TempDir()
	parityGit(t, root, "init", "--quiet")
	t.Chdir(root)
	dirs := writeParityFixture(t, root)
	log := filepath.Join(t.TempDir(), "requests")
	t.Setenv("TOKEN_LOG", log)
	for _, tc := range parityCases(root, dirs) {
		t.Run(tc.name, func(t *testing.T) {
			assertParityCase(t, root, shell, log, dirs, tc)
		})
	}
}

func TestSkillTokensTaskDispatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mise integration runs on Unix")
	}
	repo, err := filepath.Abs("../..")
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(repo, "mise.toml"))
	require.NoError(t, err)
	var config struct {
		Tasks map[string]map[string]any
	}
	require.NoError(t, toml.Unmarshal(data, &config))
	require.Equal(t, "go run ./cmd/agentrepo skill-tokens", config.Tasks["skill-tokens"]["run"])
	parityBackends(t)
	// The measured directory must sit inside the repository, because the command asks Git about it.
	//nolint:usetesting // t.TempDir() lies outside the repository, where Git refuses the path.
	dir, err := os.MkdirTemp(repo, "token-task-dispatch-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	parityWrite(t, dir, "SKILL.md", "body\n")
	log := filepath.Join(t.TempDir(), "requests")
	t.Setenv("TOKEN_LOG", log)
	t.Setenv("TOKEN_FAIL", "partial")
	t.Setenv("TOKEN_FAIL_AT", "count")
	//nolint:gosec // G204: the fixed mise executable receives a test-created directory.
	cmd := exec.CommandContext(t.Context(), "mise", "run", "skill-tokens", dir+"/")
	cmd.Dir = repo
	output, runErr := cmd.CombinedOutput()
	require.Error(t, runErr, "%s", output)
	require.NotContains(t, string(output), "PATH agentrepo invoked")
	requests, err := os.ReadFile(log)
	require.NoError(t, err, "%s", output)
	require.Equal(t, 1, strings.Count(string(requests), "ant "), string(requests))
	require.NoFileExists(t, filepath.Join(dir, "tokens.json"))
	t.Setenv("TOKEN_FAIL_AT", "")
	//nolint:gosec // G204: the fixed mise executable receives a test-created directory.
	cmd = exec.CommandContext(t.Context(), "mise", "run", "skill-tokens", dir+"/", "missing-token-task-skill/")
	cmd.Dir = repo
	output, runErr = cmd.CombinedOutput()
	require.NoError(t, runErr, "%s", output)
	require.Contains(t, string(output), "skip missing-token-task-skill: no SKILL.md")
	record, err := os.ReadFile(filepath.Join(dir, "tokens.json"))
	require.NoError(t, err)
	require.Contains(t, string(record), `"body_tokens": 5`)
	require.NotContains(t, string(output), "PATH agentrepo invoked")
}
