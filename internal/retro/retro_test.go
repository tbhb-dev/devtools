// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package retro_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	parent := filepath.Dir(path)
	base := filepath.VolumeName(parent) + string(filepath.Separator)
	if !filepath.IsAbs(parent) {
		base = "."
	}
	tree, err := os.OpenRoot(base)
	require.NoError(t, err)
	defer tree.Close()
	rel, err := filepath.Rel(base, parent)
	require.NoError(t, err)
	require.NoError(t, tree.MkdirAll(rel, 0o700))
	require.NoError(t, tree.WriteFile(filepath.Join(rel, filepath.Base(path)), []byte(body), 0o600))
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func git(t *testing.T, args ...string) {
	t.Helper()
	b, err := exec.CommandContext(t.Context(), "git", args...).CombinedOutput()
	require.NoError(t, err, "%s", b)
}

func fixture(t *testing.T) (string, string, string) {
	t.Helper()
	linkFakeUV(t)
	source, err := filepath.Abs("../../packages/agents/claude/.apm/skills/analyze-session")
	require.NoError(t, err)
	claude, codex := read(t, "testdata/claude.jsonl"), read(t, "testdata/codex.jsonl")
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "CLAUDE_CODE_SESSION_ID", "CLAUDECODE", "CODEX_THREAD_ID", "ANALYZE_SESSION_FORMAT", "ANALYZE_PREFLIGHT_LOOPS"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("DEVTOOLS_HARNESS", "claude")
	t.Chdir(t.TempDir())
	git(t, "init", "--quiet", "--initial-branch=main")
	c := filepath.Join(os.Getenv("HOME"), ".claude/projects/proj/session-claude-1.jsonl")
	x := filepath.Join(os.Getenv("HOME"), ".codex/sessions/2026/01/02/rollout-cafe.jsonl")
	write(t, c, claude)
	write(t, x, codex)
	return source, c, x
}

func run(t *testing.T, skill string, args ...string) (int, string, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	code := agentrepo.Run(append([]string{"retro", "new", "--skill-dir", skill}, args...), nil, &out, &stderr)
	return code, out.String(), stderr.String()
}

// The first three cases preserve the corresponding new-retro Bats scenarios.
func TestRetroNew(t *testing.T) {
	skill, c, x := fixture(t)
	t.Setenv("DEVTOOLS_HARNESS", "claude")
	for _, tc := range []struct{ harness, transcript, dir string }{
		{"claude", c, ".claude/retros/20260102_session-claude-1"},
		{"codex", x, ".codex/retros/20260102_019abc00-feed"},
	} {
		t.Setenv("DEVTOOLS_HARNESS", tc.harness)
		write(t, "extra.txt", "evidence")
		write(t, "extra-dir/nested", "nested")
		require.NoError(t, os.Symlink("..", "extra-dir/ancestor"))
		code, out, stderr := run(t, skill, tc.transcript, "extra.txt", "extra-dir", "absent")
		require.Zero(t, code, stderr)
		require.Contains(t, stderr, "skipping missing artifact: absent")
		lines := strings.Split(strings.TrimSpace(out), "\n")
		require.Len(t, lines, 4)
		for i, label := range []string{"retro dir:", "RETRO.md:", "artifacts:", "gitignore:"} {
			require.True(t, strings.HasPrefix(lines[i], label))
		}
		require.Contains(t, out, "4 canned reports, 2 extra")
		require.NoFileExists(t, ".gitignore")
		require.Equal(t, "evidence", read(t, tc.dir+"/artifacts/extra.txt"))
		require.Equal(t, "nested", read(t, tc.dir+"/artifacts/extra-dir/nested"))
		target, err := os.Readlink(tc.dir + "/artifacts/extra-dir/ancestor")
		require.NoError(t, err)
		require.Equal(t, "..", target)
		require.Equal(t, read(t, tc.transcript), read(t, tc.dir+"/artifacts/transcript.jsonl"))
		for _, report := range []string{"index", "errors", "loops", "lint"} {
			require.FileExists(t, tc.dir+"/artifacts/"+report+".txt")
		}
		body := read(t, tc.dir+"/RETRO.md")
		require.NotContains(t, body, "{{SESSION_ID}}")
		write(t, tc.dir+"/RETRO.md", "mine\x00\n")
		code, out, stderr = run(t, skill, tc.transcript, "extra.txt", "extra-dir")
		require.Zero(t, code, stderr)
		require.Contains(t, out, "already ignored")
		require.Contains(t, out, "left alone")
		require.Equal(t, "mine\x00\n", read(t, tc.dir+"/RETRO.md"))
		require.Equal(t, 1, strings.Count(read(t, ".git/info/exclude"), "."+tc.harness+"/retros/"))
		require.NoError(t, os.Remove("extra-dir/ancestor"))
	}
}

func TestRetroWorktreeIsolation(t *testing.T) {
	skill, c, _ := fixture(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	logical, err := filepath.EvalSymlinks(cwd)
	require.NoError(t, err)
	root := filepath.ToSlash(logical)
	git(
		t,
		"-c",
		"user.name=Fixture",
		"-c",
		"user.email=fixture@example.invalid",
		"-c",
		"commit.gpgsign=false",
		"commit",
		"--allow-empty",
		"-qm",
		"fixture",
	)
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, "worktree", "add", "--quiet", "-b", "linked", linked)
	t.Chdir(linked)
	t.Setenv("GIT_DIR", t.TempDir())
	t.Setenv("GIT_WORK_TREE", t.TempDir())
	t.Setenv("CDPATH", t.TempDir())
	write(t, filepath.Join(root, ".gitignore"), "unchanged\n")
	code, out, stderr := run(t, skill, c)
	require.Zero(t, code, stderr)
	require.Contains(t, out, root+"/.claude/retros/")
	require.Equal(t, "unchanged\n", read(t, filepath.Join(root, ".gitignore")))
	require.NoDirExists(t, ".claude/retros")
}

func stub(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "templates/RETRO.md"), "{{SESSION_ID}} {{RECORDS}} {{SIZE_MB}} {{ANALYZED}}\n")
	return dir
}

func TestRetroSymlinkedTranscript(t *testing.T) {
	skill, c, _ := fixture(t)
	t.Setenv("DEVTOOLS_HARNESS", "claude")
	write(t, "originals/transcript.jsonl", read(t, c))
	require.NoError(t, os.Symlink("originals/transcript.jsonl", "link.jsonl"))
	code, _, stderr := run(t, skill, "link.jsonl")
	require.Zero(t, code, stderr)
	require.Equal(t, read(t, c), read(t, ".claude/retros/20260102_link/artifacts/transcript.jsonl"))
}

func TestRetroFailuresAndFormat(t *testing.T) {
	_, c, _ := fixture(t)
	t.Setenv("LC_ALL", "hostile")
	t.Setenv("PYTHONUTF8", "0")
	skill := stub(t)
	code, out, stderr := run(t, skill)
	require.Equal(t, 2, code)
	require.Empty(t, out)
	require.Contains(t, stderr, "usage:")
	code, _, stderr = run(t, skill, "absent")
	require.Equal(t, 1, code)
	require.Contains(t, stderr, "new-retro: no such transcript:")
	code, _, stderr = run(t, t.TempDir(), c)
	require.Equal(t, 1, code)
	require.Contains(t, stderr, "new-retro: missing template:")
	for _, format := range []string{"", "auto", "claude", "codex"} {
		t.Setenv("ANALYZE_SESSION_FORMAT", format)
		code, _, stderr = run(t, skill, c)
		require.Zero(t, code, stderr)
	}
	for _, report := range []string{"meta", "index", "errors", "loops", "lint"} {
		t.Setenv("FAIL_REPORT", report)
		code, out, stderr = run(t, skill, c)
		require.Equal(t, 9, code)
		require.Empty(t, out)
		require.Empty(t, stderr)
	}
	t.Setenv("FAIL_REPORT", "")
	for _, meta := range []string{"SESSION_ID=stub", "SESSION_DATE=20260102"} {
		t.Setenv("RETRO_FAKE_META", meta)
		code, _, stderr = run(t, skill, c)
		require.Equal(t, 1, code)
		require.Contains(t, stderr, "could not read session metadata")
	}
	t.Chdir(t.TempDir())
	code, _, stderr = run(t, skill, c)
	require.Equal(t, 1, code)
	require.Contains(t, stderr, "new-retro: not inside a git repository")
}

func TestRetroPathFailure(t *testing.T) {
	skill, c, _ := fixture(t)
	linkFake(t, "git")
	t.Setenv(fakeGitEnv, "1")
	code, out, _ := run(t, skill, c)
	require.NotZero(t, code)
	require.Empty(t, out)
	require.NoDirExists(t, ".claude/retros")
	require.NoDirExists(t, "partial path")
}

func TestRetroHarnessPrecedence(t *testing.T) {
	skill, c, _ := fixture(t)
	t.Setenv("DEVTOOLS_HARNESS", "")
	for _, tc := range []struct{ claude, codex, dir string }{
		{"", "thread", ".codex"}, {"session", "thread", ".claude"}, {"", "", ".claude"},
	} {
		t.Setenv("CLAUDE_CODE_SESSION_ID", tc.claude)
		t.Setenv("CODEX_THREAD_ID", tc.codex)
		code, out, stderr := run(t, skill, c)
		require.Zero(t, code, stderr)
		require.Contains(t, out, tc.dir+"/retros/")
	}
	code, out, stderr := run(t, strings.Replace(filepath.ToSlash(skill), "/claude/", "/codex/", 1), c)
	require.Zero(t, code, stderr)
	require.Contains(t, out, ".codex/retros/")
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CODEX_THREAD_ID", "thread")
	code, out, stderr = run(t, skill, c)
	require.Zero(t, code, stderr)
	require.Contains(t, out, ".claude/retros/")
}

func TestRetroDefaultSkillDirectory(t *testing.T) {
	skill, c, _ := fixture(t)
	t.Setenv("DEVTOOLS_HARNESS", "")
	deployed := filepath.Join(".agents", "skills", "analyze-session")
	require.NoError(t, os.CopyFS(deployed, os.DirFS(skill)))
	t.Chdir(deployed)
	var out, stderr bytes.Buffer
	require.Zero(t, agentrepo.Run([]string{"retro", "new", c}, nil, &out, &stderr), stderr.String())
	require.Contains(t, out.String(), ".codex/retros/")
}
