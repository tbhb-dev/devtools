// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package skills_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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
	samples := make([]sample, 0, 27)
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
			status := skills.Check(root, []string{"skill/", "missing"}, &out)
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
	var out bytes.Buffer
	require.Equal(t, 1, skills.Check(root, []string{"valid", "unreadable"}, &out))
	require.Equal(
		t,
		"unreadable/SKILL.md:1 [error] unreadable  could not parse the frontmatter\nTOTAL: 1 finding(s)\n",
		out.String(),
	)
}

func TestDiscovery(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, dir := range []string{"packages/agents/claude/.apm/skills/shared", "packages/agents/codex/.apm/skills/shared", ".claude/skills/shared", ".claude/skills/local"} {
		writeManifest(t, root, dir, "body\n")
	}
	var out bytes.Buffer
	require.Equal(t, 1, skills.Check(root, nil, &out))
	require.NotContains(t, out.String(), ".claude/skills/shared")
	require.Contains(t, out.String(), ".claude/skills/local/SKILL.md")
	require.Contains(t, out.String(), "TOTAL: 6 finding(s)")
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
	status := skills.Check(root, targets, &baseline)
	for _, locale := range []string{"C", "C.UTF-8", "tr_TR.UTF-8"} {
		t.Setenv("LC_ALL", locale)
		var out bytes.Buffer
		require.Equal(t, status, skills.Check(root, targets, &out))
		require.Equal(t, baseline.String(), out.String())
	}
}
