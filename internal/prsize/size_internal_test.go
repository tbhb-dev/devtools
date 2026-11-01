// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package prsize

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
}

func repository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	runGit(t, dir, "config", "core.hooksPath", "/dev/null")
	runGit(t, dir, "commit", "--allow-empty", "-m", "base")
	runGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	runGit(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	return dir
}

func write(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "add", "--all")
	runGit(t, dir, "commit", "--allow-empty", "-m", "change")
}

func measure(t *testing.T, dir, base, head string) Result {
	t.Helper()
	r, err := Measure(t.Context(), dir, base, head)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// This integration test deliberately uses the pinned executable, not a stub.
func TestRealSCC(t *testing.T) {
	t.Parallel()
	dir := repository(t)
	write(t, dir, "main.go", "// comment\n\npackage main\nfunc main() {}\n")
	write(t, dir, "a.py", "# comment\nx = 1\n")
	write(t, dir, "README.md", "# Heading\n\ntext\n")
	write(t, dir, "binary.go", "\x00binary\n")
	commit(t, dir)
	r := measure(t, dir, "", "")
	if r.Code != 3 || r.Prose != 2 || r.Languages["Go"] != 2 || r.Languages["Python"] != 1 || r.Level != "ok" {
		t.Fatalf("unexpected report: %+v", r)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"code", "prose", "languages", "excluded", "soft", "hard", "level"} {
		if _, ok := fields[key]; !ok {
			t.Fatal("missing", key)
		}
	}
}

func TestForcedGitColor(t *testing.T) {
	t.Parallel()
	dir := repository(t)
	runGit(t, dir, "config", "color.ui", "always")
	runGit(t, dir, "config", "color.diff", "always")
	write(t, dir, "a.py", strings.Repeat("x = 1\n", 401))
	write(t, dir, "README.md", "# Heading\n\ntext\n")
	commit(t, dir)
	r := measure(t, dir, "", "")
	if r.Code != 401 || r.Prose != 2 || r.Languages["Python"] != 401 || r.Level != "soft" {
		t.Fatalf("unexpected report: %+v", r)
	}
}

func TestMergeBaseDeletionRename(t *testing.T) {
	t.Parallel()
	dir := repository(t)
	write(t, dir, "delete.py", "x = 1\n")
	write(t, dir, "rename.py", "y = 2\n")
	commit(t, dir)
	runGit(t, dir, "branch", "base")
	runGit(t, dir, "checkout", "-b", "feature")
	runGit(t, dir, "mv", "rename.py", "renamed.py")
	if err := os.Remove(filepath.Join(dir, "delete.py")); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "comments.go", "// comment\n\n")
	commit(t, dir)
	runGit(t, dir, "checkout", "base")
	write(t, dir, "base.py", "z = 3\n")
	commit(t, dir)
	r := measure(t, dir, "base", "feature")
	if r.Code != 1 || r.Prose != 0 {
		t.Fatalf("unexpected report: %+v", r)
	}
}

func TestExclusions(t *testing.T) {
	t.Parallel()
	dir := repository(t)
	paths := []string{
		"go.sum",
		"uv.lock",
		"Cargo.lock",
		"package-lock.json",
		"pnpm-lock.yaml",
		"yarn.lock",
		"mise.lock",
		"apm.lock.yaml",
		"other.lock",
		"a/skills/example/tokens.json",
		"packages/agents/claude/.apm/skills/a/a.go",
		"packages/agents/codex/.apm/skills/a/a.go",
		".claude/skills/a/a.go",
		".agents/skills/a/a.go",
		".devtools/a.go",
		"vendor/a.go",
		"a/testdata/a.go",
		"fixtures/a.go",
		"a.golden",
		"__snapshots__/a.go",
		"CHANGELOG.md",
	}
	for _, path := range paths {
		write(t, dir, path, "one\n\ntwo\n")
	}
	commit(t, dir)
	r := measure(t, dir, "", "")
	if r.Code != 0 || r.Prose != 0 || len(r.Excluded) != len(paths) {
		t.Fatalf("unexpected report: %+v", r)
	}
	for _, excluded := range r.Excluded {
		if excluded.Raw != 3 {
			t.Fatalf("raw: %+v", excluded)
		}
	}
	runGit(t, dir, "remote", "add", "origin", "https://github.com/tbhb-dev/devtools.git")
	r = measure(t, dir, "", "")
	if r.Code != 2 || len(r.Excluded) != len(paths)-1 {
		t.Fatalf("devtools payload: %+v", r)
	}
}

func TestLevels(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		code, prose int
		level       string
	}{
		{400, 0, "ok"},
		{401, 0, "soft"},
		{800, 0, "soft"},
		{801, 0, "hard"},
		{0, 1000, "ok"},
		{0, 1001, "soft"},
		{0, 2000, "soft"},
		{0, 2001, "hard"},
	} {
		r := Result{Code: tt.code, Prose: tt.prose, Soft: Limits{400, 1000}, Hard: Limits{800, 2000}}
		r.setLevel()
		if r.Level != tt.level {
			t.Fatalf("%+v: %s", tt, r.Level)
		}
	}
}

func TestProse(t *testing.T) {
	t.Parallel()
	dir := repository(t)
	for _, path := range []string{"a.md", "a.mdx", "a.txt", "a.rst"} {
		write(t, dir, path, "text\n  \n")
	}
	commit(t, dir)
	r := measure(t, dir, "", "")
	if r.Prose != 4 || r.Code != 0 {
		t.Fatalf("unexpected report: %+v", r)
	}
}

func TestEnvironmentErrors(t *testing.T) {
	dir := repository(t)
	if _, err := Measure(t.Context(), dir, "missing", "HEAD"); err == nil {
		t.Fatal("accepted bad ref")
	}
	if _, err := Measure(t.Context(), dir, "HEAD", "missing"); err == nil {
		t.Fatal("accepted bad head")
	}
	runGit(t, dir, "symbolic-ref", "--delete", "refs/remotes/origin/HEAD")
	if _, err := Measure(t.Context(), dir, "", ""); err == nil {
		t.Fatal("accepted missing default")
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := Measure(
		context.Background(),
		dir,
		"",
		"",
	); err == nil ||
		!strings.Contains(err.Error(), "aqua:boyter/scc") {
		t.Fatalf("missing pin: %v", err)
	}
}
