// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package skills

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if data, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", data, err)
	}
	for path, body := range map[string]string{
		"packages/agents/source/skills/demo/SKILL.claude.md":       "# Claude\n@include parts/body.md\nlast",
		"packages/agents/source/skills/demo/SKILL.codex.md":        "# Codex\n",
		"packages/agents/source/skills/demo/parts/body.md":         "body\n",
		"packages/agents/source/skills/demo/shared/scripts/run.sh": "echo hello\n",
	} {
		put(t, root, path, body)
	}
	return root
}

func put(t *testing.T, root, path, body string) {
	t.Helper()
	path = filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateAndCheck(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	out := "packages/agents/claude/.apm/skills/demo"
	put(t, root, out+"/tokens.json", "tokens")
	put(t, root, out+"/stale", "old")
	put(t, root, "packages/agents/codex/.apm/skills/manual/SKILL.md", "manual")
	var stdout, stderr bytes.Buffer
	if code := Run(root, false, &stdout, &stderr); code != 0 {
		t.Fatalf("%d: %s", code, &stderr)
	}
	data, err := os.ReadFile(filepath.Join(root, out, "SKILL.md"))
	if err != nil || string(data) != "# Claude\nbody\nlast\n" {
		t.Fatalf("%q: %v", data, err)
	}
	for path, want := range map[string]string{out + "/tokens.json": "tokens", "packages/agents/codex/.apm/skills/manual/SKILL.md": "manual"} {
		got, readErr := os.ReadFile(filepath.Join(root, path))
		if readErr != nil || string(got) != want {
			t.Fatalf("%s: %q %v", path, got, readErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(root, out, "stale")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal(statErr)
	}
	stdout.Reset()
	if code := Run(root, true, &stdout, &stderr); code != 0 || stdout.Len() != 0 {
		t.Fatalf("%d: %s %s", code, &stdout, &stderr)
	}
	put(t, root, out+"/extra", "extra")
	if code := Run(
		root,
		true,
		&stdout,
		&stderr,
	); code != 1 ||
		!strings.Contains(stdout.String(), out+"/extra:1 [error] generated-drift") {
		t.Fatalf("%d: %s", code, &stdout)
	}
}

var errInjected = errors.New("injected failure")

func TestFailureWithPlausibleOutput(t *testing.T) {
	t.Parallel()
	for _, op := range []string{"discovery", "read", "fingerprint", "comparison", "copy", "parent"} {
		t.Run(op, func(t *testing.T) {
			t.Parallel()
			root := fixture(t)
			g := newGenerator(root, &bytes.Buffer{})
			if err := g.generate(false); err != nil {
				t.Fatal(err)
			}
			fail := errInjected
			inject(t, g, op, fail)
			check := op != "copy" && op != "parent"
			if err := g.generate(check); !errors.Is(err, fail) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func inject(t *testing.T, g *generator, op string, fail error) {
	t.Helper()
	switch op {
	case "discovery":
		original := g.readDir
		g.readDir = func(path string) ([]os.DirEntry, error) {
			v, readErr := original(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			return v, fail
		}
	case "read":
		original := g.readFile
		g.readFile = func(path string) ([]byte, error) {
			v, readErr := original(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			return v, fail
		}
	case "fingerprint":
		original := g.fingerprint
		g.fingerprint = func(path string) (map[string]file, error) {
			v, readErr := original(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			return v, fail
		}
	case "comparison":
		g.compare = func(map[string]file, map[string]file) ([]string, error) { return nil, fail }
	case "copy":
		g.writeFile = func(string, []byte, os.FileMode) error { return fail }
	case "parent":
		g.mkdir = func(string, os.FileMode) error { return fail }
	}
}

func TestHelpersAndHarnesses(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	base := source + "/lib/"
	for name, body := range map[string]string{"lib.sh": "worktree-ready.sh", "gates.py": "gates", "agent.sh": "agent", "signing.sh": "signing", "worktree-ready.sh": "worktree"} {
		put(t, root, base+name, body)
	}
	put(t, root, source+"/skills/demo/shared/scripts/run.sh", "lib.sh")
	put(t, root, source+"/skills/demo/claude/scripts/guard.sh", "guard")
	var out, stderr bytes.Buffer
	if code := Run(root, false, &out, &stderr); code != 0 {
		t.Fatalf("%d: %s", code, &stderr)
	}
	for _, harness := range []string{"claude", "codex"} {
		for _, name := range []string{"lib.sh", "gates.py", "agent.sh", "signing.sh", "worktree-ready.sh"} {
			if _, err := os.Stat(
				filepath.Join(root, "packages/agents", harness, ".apm/skills/demo/scripts", name),
			); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := os.Stat(
		filepath.Join(root, "packages/agents/codex/.apm/skills/demo/scripts/guard.sh"),
	); !errors.Is(
		err,
		os.ErrNotExist,
	) {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, source, "skills/demo/SKILL.codex.md")); err != nil {
		t.Fatal(err)
	}
	put(t, root, "packages/agents/codex/.apm/skills/demo/SKILL.md", "untouched")
	if code := Run(root, true, &out, &stderr); code != 0 {
		t.Fatalf("%d: %s %s", code, &out, &stderr)
	}
}

func TestContractErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, path, body, want string }{
		{"include", source + "/skills/demo/SKILL.claude.md", "@include parts/missing.md\n", "generate-skills: packages/agents/source/skills/demo/SKILL.claude.md includes parts/missing.md, which does not exist\n"},
		{"collision", source + "/skills/demo/claude/scripts/run.sh", "collision", "generate-skills: scripts/run.sh exists in both shared/ and claude/ of packages/agents/source/skills/demo\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := fixture(t)
			put(t, root, tc.path, tc.body)
			var out, stderr bytes.Buffer
			if code := Run(root, false, &out, &stderr); code != 1 || stderr.String() != tc.want {
				t.Fatalf("%d: %q", code, &stderr)
			}
		})
	}
}

func TestMissingOutputAndModeDrift(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	var out, stderr bytes.Buffer
	if code := Run(root, true, &out, &stderr); code != 1 || !strings.HasSuffix(out.String(), "TOTAL: 2 finding(s)\n") {
		t.Fatalf("%d: %s", code, &out)
	}
	g := newGenerator(root, &out)
	if err := g.generate(false); err != nil {
		t.Fatal(err)
	}
	// Index modes supply the portable executable decision used on Windows.
	path := "packages/agents/claude/.apm/skills/demo/scripts/run.sh"
	g.modes = map[string]os.FileMode{path: 0o755}
	out.Reset()
	if err := g.generate(true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), path+":1 [error] generated-drift") {
		t.Fatal(out.String())
	}
}

func TestWindowsUntrackedModes(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	var out bytes.Buffer
	g := newGenerator(root, &out)
	// Simulate the index-backed mode decisions used when the filesystem
	// can't represent executable bits. Newly generated output is untracked.
	g.modes = map[string]os.FileMode{source + "/skills/demo/shared/scripts/run.sh": executableMode}
	if err := g.generate(false); err != nil {
		t.Fatal(err)
	}
	path := "packages/agents/claude/.apm/skills/demo/scripts/run.sh"
	if err := os.Chmod(g.path(path), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := g.generate(true); err != nil || g.findings != 0 {
		t.Fatalf("%v: %s", err, &out)
	}
	// An explicitly indexed mode remains authoritative and can expose drift.
	g.modes[path] = regularMode
	if err := g.generate(true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), path+":1 [error] generated-drift") {
		t.Fatal(out.String())
	}
}

func TestIndexedExecutableGeneration(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	path := source + "/skills/demo/shared/scripts/run.sh"
	info, statErr := os.Stat(filepath.Join(root, path))
	if statErr != nil {
		t.Fatal(statErr)
	}
	if chmodErr := os.Chmod(filepath.Join(root, path), info.Mode()|0o100); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	for _, cmd := range []*exec.Cmd{
		exec.CommandContext(t.Context(), "git", "-C", root, "add", "--", source),
		exec.CommandContext(t.Context(), "git", "-C", root, "update-index", "--chmod=+x", "--", path),
	} {
		if data, runErr := cmd.CombinedOutput(); runErr != nil {
			t.Fatalf("git: %s %v", data, runErr)
		}
	}
	var out, stderr bytes.Buffer
	if code := Run(root, false, &out, &stderr); code != 0 {
		t.Fatalf("%d: %s", code, &stderr)
	}
	out.Reset()
	if code := Run(root, true, &out, &stderr); code != 0 || out.Len() != 0 {
		t.Fatalf("%d: %s %s", code, &out, &stderr)
	}
}
