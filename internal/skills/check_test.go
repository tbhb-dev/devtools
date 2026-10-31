// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package skills_test

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/skills"
)

type sample struct {
	label, text string
	rules       []string
}

func corpus() []sample {
	manifest := func(name, desc string) string { return "---\nname: " + name + "\ndescription: " + desc + "\n---\n" }
	samples := make([]sample, 0, 29)
	samples = append(samples, []sample{
		{"clean", manifest("valid-name-1", "plain text"), nil},
		{"missing", "body only\n", []string{"name-missing", "description-missing"}},
		{"empty", manifest("", ""), []string{"name-missing", "description-missing"}},
		{"uppercase", manifest("Name", "text"), []string{"name-charset"}},
		{"punctuation", manifest("name_", "text"), []string{"name-charset"}},
		{"anthropic", manifest("pre-anthropic-post", "text"), []string{"name-reserved"}},
		{"claude", manifest("pre-claude-post", "text"), []string{"name-reserved"}},
		{"xml", manifest("<name>", "<text>"), []string{"name-charset", "xml-tags"}},
		{"reverse-xml", manifest("name", ">text<"), nil},
		{"split-xml", manifest("name<", "text>"), []string{"name-charset", "xml-tags"}},
		{"folded", "---\nname: valid\ndescription: >-\n  hello\n  world\nother: yes\n---\n", nil},
		{"empty-folded", "---\nname: valid\ndescription: >-\n  \nother: yes\n---\n", []string{"description-missing"}},
		{"last-folded", "---\nname: valid\ndescription: >-\n  hello\n---\n", nil},
		{"quotes", manifest("'valid'", "\"text\""), []string{"name-charset"}},
		{"crlf", strings.ReplaceAll(manifest("valid", "text"), "\n", "\r\n"), nil},
		{"unicode-space", manifest("\u2003valid\u2003", "text\u001c"), nil},
		{
			"folded-nul-separator",
			"---\nname: valid\ndescription: >-\n  " + strings.Repeat("a", 1022) + "\n  \x00\n  b\nother: yes\n---\n",
			[]string{"description-too-long"},
		},
		{"nul-after-space", manifest("valid", strings.Repeat("a", 1024)+" \x00"), []string{"description-too-long"}},
	}...)
	for _, n := range []int{63, 64, 65} {
		var rules []string
		if n > 64 {
			rules = append(rules, "name-too-long")
		}
		samples = append(samples, sample{fmt.Sprintf("name-%d", n), manifest(strings.Repeat("a", n), "text"), rules})
		rules = []string{"name-charset"}
		if n > 64 {
			rules = append([]string{"name-too-long"}, rules...)
		}
		samples = append(
			samples,
			sample{
				fmt.Sprintf("utf8-name-%d", n),
				manifest(strings.Repeat("é", n/2)+strings.Repeat("a", n%2), "text"),
				rules,
			},
		)
	}
	for _, n := range []int{1023, 1024, 1025} {
		var rules []string
		if n > 1024 {
			rules = []string{"description-too-long"}
		}
		samples = append(
			samples,
			sample{fmt.Sprintf("desc-%d", n), manifest("valid", strings.Repeat("a", n)), rules},
			sample{
				fmt.Sprintf("utf8-desc-%d", n),
				manifest("valid", strings.Repeat("é", n/2)+strings.Repeat("a", n%2)),
				rules,
			},
		)
	}
	return samples
}

func writeManifest(t *testing.T, root, dir, text string) {
	t.Helper()
	path := filepath.Join(root, dir)
	require.NoError(t, os.MkdirAll(path, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(text), 0o600))
}

func TestCorpus(t *testing.T) {
	t.Parallel()
	for _, item := range corpus() {
		t.Run(item.label, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeManifest(t, root, "skill", item.text)
			var out bytes.Buffer
			status := skills.Check(t.Context(), root, []string{"skill/", "missing"}, &out, io.Discard)
			var rules []string
			for line := range strings.SplitSeq(out.String(), "\n") {
				if strings.Contains(line, " [error] ") {
					rules = append(rules, strings.Fields(line)[2])
				}
			}
			require.Equal(t, item.rules, rules)
			if len(rules) == 0 {
				require.Zero(t, status)
				require.Empty(t, out.String())
			} else {
				require.Equal(t, 1, status)
				require.Contains(t, out.String(), fmt.Sprintf("TOTAL: %d finding(s)\n", len(rules)))
			}
		})
	}
}

func TestUnreadableAfterValid(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeManifest(t, root, "valid", "---\nname: valid\ndescription: text\n---\n")
	writeManifest(t, root, "unreadable", "\xff")
	writeManifest(t, root, "also-unreadable", "\xff")
	var out bytes.Buffer
	require.Equal(
		t,
		1,
		skills.Check(t.Context(), root, []string{"valid", "unreadable", "also-unreadable"}, &out, io.Discard),
	)
	require.Equal(
		t,
		"unreadable/SKILL.md:1 [error] unreadable  could not parse the frontmatter\n"+
			"also-unreadable/SKILL.md:1 [error] unreadable  could not parse the frontmatter\nTOTAL: 2 finding(s)\n",
		out.String(),
	)
}

func TestDiscovery(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, dir := range []string{
		"packages/agents/claude/.apm/skills/shared", "packages/agents/codex/.apm/skills/shared",
		".claude/skills/shared", ".claude/skills/local", ".claude/skills/.hidden",
		"packages/agents/codex/.apm/skills/.hidden", "packages/agents/.hidden/.apm/skills/local",
	} {
		writeManifest(t, root, dir, "body\n")
	}
	var out bytes.Buffer
	require.Equal(t, 1, skills.Check(t.Context(), root, nil, &out, io.Discard))
	require.NotContains(t, out.String(), ".claude/skills/shared")
	require.Contains(t, out.String(), ".claude/skills/local/SKILL.md")
	require.Contains(t, out.String(), "TOTAL: 6 finding(s)")
	require.NotContains(t, out.String(), ".hidden")
	var explicit bytes.Buffer
	require.Equal(t, 1, skills.Check(t.Context(), root, []string{".claude/skills/.hidden"}, &explicit, io.Discard))
	require.Contains(t, explicit.String(), ".claude/skills/.hidden/SKILL.md:1")
}

func TestLocaleIndependence(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	root := t.TempDir()
	targets := make([]string, 0, len(corpus()))
	for _, item := range corpus() {
		writeManifest(t, root, item.label, item.text)
		targets = append(targets, item.label)
	}
	var baseline bytes.Buffer
	status := skills.Check(t.Context(), root, targets, &baseline, io.Discard)
	for _, locale := range []string{"C", "C.UTF-8", "tr_TR.UTF-8"} {
		t.Setenv("LC_ALL", locale)
		var out bytes.Buffer
		require.Equal(t, status, skills.Check(t.Context(), root, targets, &out, io.Discard))
		require.Equal(t, baseline.String(), out.String())
	}
}

// TestInvalidUTF8 pins the one deliberate difference from the frozen shell:
// stdout and status match, but Go omits the old Python error dump.
func TestInvalidUTF8(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeManifest(t, root, "unreadable", "\xff")
	var out, stderr bytes.Buffer
	require.Equal(t, 1, skills.Check(t.Context(), root, []string{"unreadable"}, &out, &stderr))
	require.Equal(
		t,
		"unreadable/SKILL.md:1 [error] unreadable  could not parse the frontmatter\nTOTAL: 1 finding(s)\n",
		out.String(),
	)
	require.Empty(t, stderr.String(), "the old Python error dump is deliberately omitted")
}

func TestExplicitTargets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeManifest(t, root, "first", "body\n")
	writeManifest(t, root, "second", "body\n")
	targets := []string{"second//", "first/../first", "missing"}
	// Windows resolves the dots before it looks at the disk, so a missing
	// directory in front of them doesn't hide the manifest there.
	if runtime.GOOS != "windows" {
		targets = []string{"second//", "missing/../first", "first/../first", "missing"}
	}
	original := slices.Clone(targets)
	var out bytes.Buffer
	require.Equal(t, 1, skills.Check(t.Context(), root, targets, &out, io.Discard))
	var want strings.Builder
	for _, target := range []string{"second/", "first/../first"} {
		for _, field := range []string{"name", "description"} {
			_, _ = fmt.Fprintf(
				&want,
				"%s/SKILL.md:1 [error] %s-missing  the frontmatter sets no %s\n",
				target,
				field,
				field,
			)
		}
	}
	want.WriteString("TOTAL: 4 finding(s)\n")
	require.Equal(t, want.String(), out.String())
	require.Equal(t, original, targets)
}

func TestFilesystemOnly(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()
	for _, item := range []sample{{"valid", "---\nname: valid\ndescription: text\n---\n", nil}, {"unreadable", "\xff", nil}} {
		writeManifest(t, root, item.label, item.text)
	}
	before := make(map[string]string)
	record := func(state map[string]string) {
		t.Helper()
		require.NoError(t, fs.WalkDir(os.DirFS(root), ".", func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			state[path] = entry.Type().String()
			if !entry.IsDir() {
				data, readErr := os.ReadFile(filepath.Join(root, path))
				if readErr != nil {
					return fmt.Errorf("read manifest snapshot: %w", readErr)
				}
				state[path] += string(data)
			}
			return nil
		}))
	}
	record(before)
	var out, stderr bytes.Buffer
	require.Equal(t, 1, skills.Check(t.Context(), root, []string{"valid", "unreadable"}, &out, &stderr))
	require.Empty(t, stderr.String())
	after := make(map[string]string)
	record(after)
	require.Equal(t, before, after, "checking with no commands on PATH preserves the filesystem")
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
	require.Equal(t, 1, skills.Check(t.Context(), root, []string{"valid", "unreadable"}, &out, io.Discard))
	require.Equal(
		t,
		"unreadable/SKILL.md:1 [error] unreadable  could not parse the frontmatter\nTOTAL: 1 finding(s)\n",
		out.String(),
	)
}
