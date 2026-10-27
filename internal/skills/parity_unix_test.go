//go:build unix

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package skills_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/skills"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "git", "init", "--quiet", root)
	data, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", data)
	return root
}

func snapshot(t *testing.T, root string) string {
	t.Helper()
	tree, openErr := os.OpenRoot(root)
	require.NoError(t, openErr)
	defer tree.Close()
	var state strings.Builder
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk snapshot: %w", err)
		}
		if entry.IsDir() {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("snapshot path: %w", relErr)
		}
		var data []byte
		var readErr error
		if entry.Type()&os.ModeSymlink != 0 {
			var target string
			target, readErr = tree.Readlink(relative)
			data = []byte(target)
		} else {
			data, readErr = tree.ReadFile(relative)
		}
		if readErr != nil {
			return fmt.Errorf("read snapshot: %w", readErr)
		}
		_, writeErr := fmt.Fprintf(&state, "%s %x\n", path, sha256.Sum256(data))
		if writeErr != nil {
			return fmt.Errorf("write snapshot: %w", writeErr)
		}
		return nil
	}))
	return state.String()
}

func parity(t *testing.T, root string, targets []string) {
	t.Helper()
	script, err := filepath.Abs("../../tools/skill-limits-legacy.sh")
	require.NoError(t, err)
	before := snapshot(t, root)
	locales := []string{"C"}
	available, localeErr := exec.CommandContext(t.Context(), "locale", "-a").Output()
	require.NoError(t, localeErr)
	for locale := range strings.FieldsSeq(string(available)) {
		if strings.Contains(strings.ToLower(locale), "utf") {
			locales = append(locales, locale)
			break
		}
	}
	for _, locale := range locales {
		cmd := exec.CommandContext(t.Context(), "bash", script)
		cmd.Args = append(cmd.Args, targets...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "LC_ALL="+locale)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		shellCode := 0
		if runErr := cmd.Run(); runErr != nil {
			var exit *exec.ExitError
			require.ErrorAs(t, runErr, &exit)
			shellCode = exit.ExitCode()
		}
		var goOut, goErr bytes.Buffer
		goCode := skills.Run(t.Context(), root, targets, &goOut, &goErr)
		require.Equal(t, shellCode, goCode)
		require.Equal(t, stdout.String(), goOut.String())
		require.Equal(t, stderr.String(), goErr.String())
		require.Equal(t, before, snapshot(t, root), "AC11: repository bytes, index and refs")
	}
}

func TestFrozenParity(t *testing.T) {
	t.Parallel()
	for _, item := range corpus() {
		t.Run(item.label, func(t *testing.T) {
			t.Parallel()
			root := fixture(t)
			writeManifest(t, root, "skill", item.text)
			parity(t, root, []string{"missing/", "skill//", "skill"})
		})
	}
}

func TestDiscoveryParity(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	for _, dir := range []string{"packages/agents/codex/.apm/skills/z", "packages/agents/claude/.apm/skills/a", ".claude/skills/a", ".claude/skills/local"} {
		writeManifest(t, root, dir, "body\n")
	}
	parity(t, root, nil)
	parity(t, root, []string{".claude/skills/local", "missing", "packages/agents/claude/.apm/skills/a/"})
}

func TestHiddenDiscoveryParity(t *testing.T) {
	t.Parallel()
	for _, hidden := range []string{
		".claude/skills/.hidden",
		"packages/agents/codex/.apm/skills/.hidden",
		"packages/agents/.hidden/.apm/skills/local",
	} {
		t.Run(hidden, func(t *testing.T) {
			t.Parallel()
			root := fixture(t)
			writeManifest(t, root, hidden, "body\n")
			writeManifest(t, root, ".claude/skills/local", "body\n")
			parity(t, root, nil)
			// Explicit targets still include hidden directories.
			parity(t, root, []string{hidden})
		})
	}
}

func TestExplicitTraversalParity(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	writeManifest(t, root, "skill", "---\nname: valid\ndescription: text\n---\n")
	writeManifest(t, root, "other/skill", "body\n")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "other/child"), 0o700))
	require.NoError(t, os.Symlink("other/child", filepath.Join(root, "link")))
	for _, target := range []string{"link/../skill", "missing/../other/skill"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			parity(t, root, []string{target})
			parity(t, root, []string{root + "/" + target})
		})
	}
}

func TestOfflineReadOnly(t *testing.T) {
	root := fixture(t)
	bin := t.TempDir()
	git, err := exec.LookPath("git")
	require.NoError(t, err)
	t.Setenv("SKILL_LIMIT_REAL_GIT", git)
	trap := filepath.Join(t.TempDir(), "trap")
	t.Setenv("SKILL_LIMIT_TRAP", trap)
	source := filepath.Join(t.TempDir(), "trap.go")
	require.NoError(t, os.WriteFile(source, []byte(`package main
import ("os"; "os/exec"; "path/filepath"; "strings")
func main() {
	if filepath.Base(os.Args[0]) == "git" && strings.Join(os.Args[1:], " ") == "rev-parse --show-toplevel" {
	c := exec.Command(os.Getenv("SKILL_LIMIT_REAL_GIT"), os.Args[1:]...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if c.Run() != nil { os.Exit(98) }; return
	}
	_ = os.WriteFile(os.Getenv("SKILL_LIMIT_TRAP"), []byte("unexpected command"), 0600)
	os.Exit(99)
}`), 0o600))
	executable := filepath.Join(bin, "git")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", executable, source)
	data, buildErr := build.CombinedOutput()
	require.NoError(t, buildErr, "%s", data)
	for _, tool := range []string{"curl", "wget", "ssh", "gh", "python3"} {
		require.NoError(t, os.Link(executable, filepath.Join(bin, tool)))
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, text := range []string{"---\nname: valid\ndescription: text\n---\n", "body\n", "\xff"} {
		writeManifest(t, root, "skill", text)
		before := snapshot(t, root)
		var out, stderr bytes.Buffer
		skills.Run(t.Context(), root, []string{"skill"}, &out, &stderr)
		require.Empty(t, stderr.String())
		require.Equal(t, before, snapshot(t, root))
		require.NoFileExists(t, trap)
	}
}

func TestReadFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeManifest(t, root, "valid", "---\nname: valid\ndescription: text\n---\n")
	writeManifest(t, root, "unreadable", "body\n")
	file := filepath.Join(root, "unreadable/SKILL.md")
	require.NoError(t, os.Chmod(file, 0))
	t.Cleanup(func() { require.NoError(t, os.Chmod(file, 0o600)) })
	if _, err := os.ReadFile(file); err == nil {
		t.Skip("process can read permission-denied manifests")
	}
	var out bytes.Buffer
	require.Equal(t, 1, skills.Check(root, []string{"valid", "unreadable"}, &out))
	require.Equal(
		t,
		"unreadable/SKILL.md:1 [error] unreadable  could not parse the frontmatter\nTOTAL: 1 finding(s)\n",
		out.String(),
	)
}
