// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package testaffected

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
)

func TestPathMap(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ path, want string }{
		{"internal/buildmeta/buildmeta.go", "test-go,test-hooks"},
		{"internal/buildmeta/buildmeta_test.go", "test-go"},
		{"cmd/agy-otel/main.go", "test-go,test-hooks"},
		{"packages/devtools/src/devtools/buildmeta.py", "test-py"},
		{"packages/devtools/tests/conftest.py", "test-full"},
		{"internal/markdown/detect.go", "test-full"},
		{"cmd/guard-markdown/main.go", "test-full"},
		{"README.md", ""},
		{"docs/compatibility.md", ""},
		{"unknown.file", "test-full"},
		{"docs/image.png", "test-full"},
		{"mise.toml", "test-full"},
		{"go.mod", "test-full"},
		{".config/mise/conf.d/tasks.toml", "test-full"},
		{".devtools/tasks/test.toml", "test-full"},
		{".devtools/scripts/check-leaks.sh", "test-full"},
		{"tests/test_helper.bash", "test-full"},
		{"tools/fuzz.sh", "test-full"},
		{"internal/testaffected/select.go", "test-full"},
		{"internal/testaffected/select_test.go", "test-full"},
		{"cmd/test-affected/main.go", "test-full"},
		{"internal/bats/run.go", "test-full"},
		{"cmd/test-hooks/main.go", "test-full"},
		{"internal/agentrepo/diagnose.go", "test-full"},
		{"packages/agents/source/lib/lib.sh", "test-full"},
		{"packages/agents/source/skills/commit/shared/scripts/commit.sh", "test-full"},
		{"packages/agents/source/skills/pr/shared/scripts/create-pr.sh", "test-full"},
		{".github/workflows/ci.yml", "test-full"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, strings.Join(selectPaths([]string{tc.path}), ","))
		})
	}
	require.Equal(t, []string{"test-go", "test-py", "test-hooks"}, selectPaths([]string{
		"internal/buildmeta/buildmeta.go",
		"packages/devtools/src/devtools/buildmeta.py",
		"internal/buildmeta/buildmeta.go",
	}))
	require.Equal(t, []string{"test-full"}, selectPaths([]string{"README.md", "unknown.file"}))
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = fixtureEnv()
	data, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", data)
	return strings.TrimSpace(string(data))
}

func write(t *testing.T, dir, name, data string) {
	t.Helper()
	name = filepath.Join(dir, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o700))
	require.NoError(t, os.WriteFile(name, []byte(data), 0o600))
}

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	for key, value := range map[string]string{
		"user.name": "Tester", "user.email": "tester@example.invalid", "commit.gpgsign": "false",
		"tag.gpgsign": "false", "core.hooksPath": filepath.Join(dir, "no-hooks"),
	} {
		git(t, dir, "config", key, value)
	}
	write(t, dir, "README.md", "Initial\n")
	git(t, dir, "add", "README.md")
	git(t, dir, "commit", "-qm", "Initial")
	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	git(t, dir, "checkout", "-qb", "topic")
	write(t, dir, ".git/info/exclude", "PR_AGENTDESC.md\n")
	return dir
}

func fixtureEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	return append(env, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
}

func TestStagedCommittedAndDraftBase(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"internal/buildmeta/buildmeta_test.go", "packages/devtools/src/devtools/a.py", "docs/guide.md"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			dir := fixture(t)
			write(t, dir, path, "change\n")
			git(t, dir, "add", path)
			staged, err := changedPaths(t.Context(), dir)
			require.NoError(t, err)
			require.Equal(t, []string{path}, staged)
			git(t, dir, "commit", "-qm", "Change")
			committed, err := changedPaths(t.Context(), dir)
			require.NoError(t, err)
			require.Equal(t, staged, committed)
			// Origin wins over the stale local copy of a non-default base.
			git(t, dir, "update-ref", "refs/remotes/origin/release", "HEAD")
			git(t, dir, "branch", "release", "HEAD~1")
			write(t, dir, "PR_AGENTDESC.md", "---\nbase: release\n---\n# Draft\n")
			write(t, dir, ".git/info/exclude", "PR_AGENTDESC.md\n")
			paths, err := changedPaths(t.Context(), dir)
			require.NoError(t, err)
			require.Empty(t, paths)
		})
	}
}

func TestUncertainContextFallsBack(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"no-base", "bad-draft", "missing-base", "revision-expression", "untracked", "unstaged", "no-merge-base", "unborn", "not-repository"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			dir := fixture(t)
			switch mode {
			case "no-base":
				git(t, dir, "symbolic-ref", "--delete", "refs/remotes/origin/HEAD")
				git(t, dir, "update-ref", "-d", "refs/remotes/origin/main")
				git(t, dir, "branch", "-D", "main")
			case "bad-draft":
				write(t, dir, "PR_AGENTDESC.md", "---\nbase: [invalid]\n---\n")
			case "missing-base":
				write(t, dir, "PR_AGENTDESC.md", "---\nbase: absent\n---\n")
			case "revision-expression":
				write(t, dir, "PR_AGENTDESC.md", "---\nbase: main~0\n---\n")
			case "untracked":
				write(t, dir, "README-new.md", "change")
			case "unstaged":
				write(t, dir, "README.md", "change")
			case "no-merge-base":
				git(t, dir, "checkout", "--orphan", "unrelated")
				git(t, dir, "commit", "-qm", "Unrelated")
			case "unborn":
				git(t, dir, "update-ref", "-d", "HEAD")
			case "not-repository":
				dir = t.TempDir()
			}
			var out bytes.Buffer
			var tasks []string
			require.NoError(
				t,
				execute(t.Context(), dir, &out, func(task string) error { tasks = append(tasks, task); return nil }),
			)
			require.Equal(t, []string{"test-full"}, tasks)
			require.Contains(t, out.String(), "test-full")
		})
	}
}

func TestEmptyAndFailure(t *testing.T) {
	t.Parallel()
	dir := fixture(t)
	var out bytes.Buffer
	calls := 0
	run := func(string) error { calls++; return errors.ErrUnsupported }
	require.NoError(t, execute(t.Context(), dir, &out, run))
	require.Contains(t, out.String(), "No affected tests selected")
	require.Zero(t, calls)
	write(t, dir, "internal/buildmeta/buildmeta_test.go", "change")
	git(t, dir, "add", "internal/buildmeta/buildmeta_test.go")
	require.ErrorIs(t, execute(t.Context(), dir, &out, run), errors.ErrUnsupported)
	require.Equal(t, 1, calls)
}

func TestTaskPreservation(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../mise.toml")
	require.NoError(t, err)
	var config struct {
		Tasks map[string]struct {
			Run any `toml:"run"`
		} `toml:"tasks"`
	}
	require.NoError(t, toml.Unmarshal(data, &config))
	want := []any{
		"mise run check-generated-skills",
		"mise run test-go",
		"mise run test-py",
		"mise run test-hooks",
		"mise run test-leaks",
	}
	require.Equal(t, want, config.Tasks["test-full"].Run)
	require.Equal(t, "go run ./cmd/test-affected", config.Tasks["test"].Run)
	require.Contains(t, config.Tasks["check"].Run, "mise run test-full")
	require.Contains(t, config.Tasks["lint"].Run, "mise run check-generated-skills")
}

func TestRenamesDeletionsAndNULPaths(t *testing.T) {
	t.Parallel()
	dir := fixture(t)
	old := "internal/buildmeta/buildmeta_test.go"
	write(t, dir, old, "tracked")
	git(t, dir, "add", old)
	git(t, dir, "commit", "-qm", "Before rename")
	before := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "update-ref", "refs/remotes/origin/main", before)
	write(t, dir, "docs/new.md", "tracked")
	git(t, dir, "rm", old)
	git(t, dir, "add", "docs/new.md")
	paths, err := changedPaths(t.Context(), dir)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{old, "docs/new.md"}, paths)
	require.Equal(t, []string{"test-go"}, selectPaths(paths))
	git(t, dir, "commit", "-qm", "Rename")
	paths, err = changedPaths(t.Context(), dir)
	require.NoError(t, err)
	require.Equal(t, []string{"test-go"}, selectPaths(paths))
	require.Equal(
		t,
		[]string{"docs/with\nnewline.md", "unknown.file"},
		splitPaths("docs/with\nnewline.md\x00unknown.file\x00"),
	)
}

func TestInfrastructureDiffsRunFull(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"mise.toml", ".devtools/tasks/go.toml", "internal/bats/run.go",
		"cmd/test-hooks/main.go", "internal/testaffected/select.go",
		"cmd/test-affected/main.go", ".devtools/scripts/check-leaks.sh",
		"packages/agents/source/lib/lib.sh",
		"packages/agents/source/skills/commit/shared/scripts/commit.sh",
		"packages/agents/source/skills/pr/shared/scripts/create-pr.sh",
		"unknown.file",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			dir := fixture(t)
			write(t, dir, path, "change")
			git(t, dir, "add", path)
			var tasks []string
			require.NoError(t, execute(t.Context(), dir, &bytes.Buffer{}, func(task string) error {
				tasks = append(tasks, task)
				return nil
			}))
			require.Equal(t, []string{"test-full"}, tasks)
		})
	}
}

func TestFrozenGateCalls(t *testing.T) {
	t.Parallel()
	for _, file := range []string{
		"commit/shared/scripts/commit.sh", "pr/shared/scripts/create-pr.sh",
	} {
		data, err := os.ReadFile("../../packages/agents/source/skills/" + file)
		require.NoError(t, err)
		require.Contains(t, string(data), "dt_gate test \"\" \"$rerun_gates\" mise run test")
	}
}

func TestDraftParsing(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"", "---\nbase: main\n", "---\nbase: [main]\n---", "---\nbase: main\nbase: release\n---", "---\nlabels: []\n---",
	} {
		_, err := draftBase([]byte(text))
		require.Error(t, err)
	}
	base, err := draftBase([]byte("---\r\nbase: release\r\n---\r\n# Draft"))
	require.NoError(t, err)
	require.Equal(t, "release", base)
}

func TestConflictedIndexRunsFull(t *testing.T) {
	t.Parallel()
	dir := fixture(t)
	write(t, dir, "README.md", "Topic\n")
	git(t, dir, "add", "README.md")
	git(t, dir, "commit", "-qm", "Topic")
	git(t, dir, "checkout", "main")
	write(t, dir, "README.md", "Main\n")
	git(t, dir, "add", "README.md")
	git(t, dir, "commit", "-qm", "Main")
	git(t, dir, "checkout", "topic")
	merge := exec.CommandContext(
		t.Context(),
		"git",
		"-C",
		dir,
		"merge",
		"main",
	)
	merge.Env = fixtureEnv()
	require.Error(t, merge.Run())
	var tasks []string
	require.NoError(t, execute(t.Context(), dir, &bytes.Buffer{}, func(task string) error {
		tasks = append(tasks, task)
		return nil
	}))
	require.Equal(t, []string{"test-full"}, tasks)
}
