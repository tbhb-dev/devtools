// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
	"github.com/tbhb-dev/devtools/internal/prsize"
)

func TestSizeCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	git("config", "commit.gpgsign", "false")
	git("config", "core.hooksPath", "/dev/null")
	git("commit", "--allow-empty", "-m", "base")
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	for _, tt := range []struct {
		lines int
		level string
	}{{1, "ok"}, {401, "soft"}, {801, "hard"}} {
		if err := os.WriteFile("a.py", []byte(strings.Repeat("x = 1\n", tt.lines)), 0o600); err != nil {
			t.Fatal(err)
		}
		git("add", "--", "a.py")
		git("commit", "-m", "change")
		for _, format := range []string{"text", "json"} {
			checkSize(t, format, tt.level, tt.lines)
		}
	}
	for _, args := range [][]string{{"--base", "missing"}, {"--format", "xml"}, {"--bad"}, {"extra"}} {
		var out bytes.Buffer
		if status := agentrepo.Run(append([]string{"size"}, args...), nil, &out, &out); status != 2 {
			t.Fatalf("%v: %d: %s", args, status, &out)
		}
	}
	t.Setenv("PATH", t.TempDir())
	var out bytes.Buffer
	if status := agentrepo.Run(
		[]string{"size"},
		nil,
		&out,
		&out,
	); status != 2 ||
		!strings.Contains(out.String(), "aqua:boyter/scc") {
		t.Fatalf("missing scc: %d: %s", status, &out)
	}
}

func checkSize(t *testing.T, format, level string, lines int) {
	t.Helper()
	var out, stderr bytes.Buffer
	status := agentrepo.Run([]string{"size", "--format", format}, nil, &out, &stderr)
	if status != 0 {
		t.Fatalf("status %d: %s", status, &stderr)
	}
	if format == "text" {
		if !strings.Contains(out.String(), "level: "+level) {
			t.Fatal(&out)
		}
		return
	}
	var r prsize.Result
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.Code != lines || r.Level != level {
		t.Fatalf("report: %+v", r)
	}
}

func TestSizeCheckCommand(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Chdir(t.TempDir())
	git := func(args ...string) {
		t.Helper()
		out, err := exec.CommandContext(t.Context(), "git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	git("config", "commit.gpgsign", "false")
	git("config", "core.hooksPath", "/dev/null")
	git("commit", "--allow-empty", "-m", "base")
	for _, tt := range []struct {
		path        string
		lines, want int
	}{
		{"a.py", 800, 0}, {"a.py", 801, 1}, {"a.md", 2000, 0}, {"a.md", 2001, 1},
	} {
		git("reset", "--hard", "main")
		git("checkout", "-B", "topic", "main")
		if err := os.WriteFile(tt.path, []byte(strings.Repeat("x = 1\n", tt.lines)), 0o600); err != nil {
			t.Fatal(err)
		}
		git("add", "--", tt.path)
		git("commit", "-m", "chore: measured")
		var out bytes.Buffer
		status := agentrepo.Run([]string{"size", "check", "--base", "main", "--head", "HEAD"}, nil, &out, &out)
		if status != tt.want {
			t.Fatalf("%s/%d: status %d: %s", tt.path, tt.lines, status, &out)
		}
	}
}

func TestSizeCheckDraftBase(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Chdir(t.TempDir())
	git := func(args ...string) {
		t.Helper()
		out, err := exec.CommandContext(t.Context(), "git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	git("config", "commit.gpgsign", "false")
	git("config", "core.hooksPath", "/dev/null")
	git("commit", "--allow-empty", "-m", "base")
	git("checkout", "-b", "topic")
	if err := os.WriteFile("a.py", []byte(strings.Repeat("x = 1\n", 801)), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "a.py")
	git("commit", "-m", "feat: stack base")
	git("update-ref", "refs/remotes/origin/stack", "HEAD")
	git("branch", "stack", "main") // origin's copy must win over the stale local ref.
	if err := os.WriteFile("b.py", []byte("x = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "b.py")
	git("commit", "-m", "feat: stack tip")
	for _, tt := range []struct {
		draft  string
		want   int
		counts string
	}{
		{"---\nbase: stack\n---\n# feat: tip\n", 0, "1 code SLOC"},
		{"---\nbase: main\n---\n# feat: tip\n", 1, "802 code SLOC"},
		{"---\nbase: [invalid]\n---\n", 2, ""},
		{"---\ndraft: true\n---\n", 2, ""},
	} {
		if err := os.WriteFile("draft.md", []byte(tt.draft), 0o600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		status := agentrepo.Run([]string{"size", "check", "--base", "main", "--draft", "draft.md"}, nil, &out, &out)
		if status != tt.want || !strings.Contains(out.String(), tt.counts) {
			t.Fatalf("status %d: %s", status, &out)
		}
	}
}
