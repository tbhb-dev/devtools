// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package tokens

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T) (*measurement, *bytes.Buffer, *bytes.Buffer, string) {
	t.Helper()
	root := t.TempDir()
	var out, stderr bytes.Buffer
	m := &measurement{
		root:     root,
		out:      &out,
		stderr:   &stderr,
		claude:   "claude-opus-5",
		codex:    "gpt-5.6-sol",
		encoding: "o200k_base",
	}
	dir := "packages/agents/codex/.apm/skills/example"
	require.NoError(t, os.MkdirAll(m.path(dir), 0o750))
	require.NoError(
		t,
		os.WriteFile(
			m.path(dir+"/SKILL.md"),
			[]byte("---\nname: example\ndescription: >-\n  Fold this\n  description.\n---\n\nBody café.\n"),
			0o600,
		),
	)
	m.run = func(name string, _ []string, input string) ([]byte, error) {
		if name == "git" {
			return nil, nil
		}
		require.Equal(t, "uv", name, "Codex must never invoke ant")
		return fmt.Appendf(nil, "%d\n", len(input)), nil
	}
	return m, &out, &stderr, dir
}

func TestPartition(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ input, fm, desc, body string }{
		{"plain\n", "", "", "plain\n"},
		{"---\ndescription: >-\n  a\x1cb\n---\nbody", "description: >-\n  a\x1cb", "a b", "body"},
		{"", "", "", ""},
		{"---\nname: x\ndescription: >-\n  a\n  café\nother: x\n---\nBody", "name: x\ndescription: >-\n  a\n  café\nother: x", "a café", "Body"},
		{"---\ndescription: >-\n  last\n---\n", "description: >-\n  last", "last", ""},
		{"---\ndescription: inline\n---\nbody", "description: inline", "", "body"},
	} {
		fm, desc, body := partition(tt.input)
		require.Equal(t, tt.fm, fm)
		require.Equal(t, tt.desc, desc)
		require.Equal(t, tt.body, body)
	}
}

func TestSelectionAndDeterminism(t *testing.T) {
	t.Parallel()
	m, out, stderr, dir := fixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(m.path(dir)), "plain-file"), nil, 0o600))
	require.NoError(t, m.measure(nil))
	record, err := os.ReadFile(m.path(dir + "/tokens.json"))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(record, &doc))
	require.Equal(t, "gpt-5.6-sol", doc["model"])
	require.Equal(t, "o200k_base", doc["tokenizer"])
	require.Contains(t, string(record), `"description_tokens": 22`)
	require.Contains(t, out.String(), "example                  description")
	require.NoError(t, m.measure([]string{dir + "/", "missing"}))
	again, err := os.ReadFile(m.path(dir + "/tokens.json"))
	require.NoError(t, err)
	require.Equal(t, record, again)
	require.Equal(t, "skip missing: no SKILL.md\n", stderr.String())
	for _, path := range []string{dir, m.path(dir), ".agents/skills/x", "/repo/.agents/skills/x"} {
		require.True(t, isCodex(filepath.ToSlash(path)))
	}
	for _, path := range []string{".claude/skills/x", "packages/agents/claude/.apm/skills/x", "custom"} {
		require.False(t, isCodex(path))
	}
}

func TestClaudeEnvelope(t *testing.T) {
	t.Parallel()
	m, out, _, dir := fixture(t)
	calls := 0
	m.run = func(name string, args []string, input string) ([]byte, error) {
		if name == "git" {
			return nil, nil
		}
		require.Equal(t, "ant", name)
		require.Equal(t, []string{"messages", "count-tokens"}, args)
		require.True(t, strings.HasPrefix(input, "model: claude-opus-5\nmessages:\n"))
		calls++
		if calls == 1 {
			require.Contains(t, input, "      x\n")
			return []byte(`{"input_tokens":8}`), nil
		}
		return []byte(`{"input_tokens":17}`), nil
	}
	// Relative spellings outside the packaged Codex path select Claude.
	target := "custom"
	require.NoError(t, os.Rename(m.path(dir), m.path(target)))
	require.NoError(t, m.measure([]string{target, target}))
	require.Equal(t, 9, calls)
	require.Equal(t, 1, strings.Count(out.String(), "request envelope 7 tokens"))
	record, err := os.ReadFile(m.path(target + "/tokens.json"))
	require.NoError(t, err)
	require.Contains(t, string(record), `"tokens": 10`)
	require.NotContains(t, string(record), "tokenizer")
}

func TestFailureBoundary(t *testing.T) {
	t.Parallel()
	for _, failAt := range []int{1, 2, 3, 4, 5, 6} {
		for _, response := range []string{"", "10\n", "invalid\n", "-1\n", "9223372036854775808\n"} {
			t.Run(fmt.Sprintf("%d/%q", failAt, response), func(t *testing.T) {
				t.Parallel()
				m, _, _, dir := fixture(t)
				require.NoError(t, os.WriteFile(m.path(dir+"/extra"), []byte("extra"), 0o600))
				require.NoError(t, os.WriteFile(m.path(dir+"/tokens.json"), []byte("preserved"), 0o600))
				m.run = failingProducer(failAt, response, dir)

				// Invalid discovery output is a failed file read. Preserve the record.
				require.Error(t, m.measure([]string{dir}))
				record, err := os.ReadFile(m.path(dir + "/tokens.json"))
				require.NoError(t, err)
				require.Equal(t, "preserved", string(record))
			})
		}
	}
}

func TestBundledFilesAndBudget(t *testing.T) {
	t.Setenv("BODY_BUDGET", "5000")
	m, out, stderr, dir := fixture(t)
	for name, data := range map[string]string{"a.txt": "text", "empty": "", "binary": "a\x00b", "é😀": "unicode"} {
		require.NoError(t, os.WriteFile(m.path(dir+"/"+name), []byte(data), 0o600))
	}
	m.run = func(name string, _ []string, _ string) ([]byte, error) {
		if name == "git" {
			return []byte(
				dir + "/é😀\n" + dir + "/tokens.json\n" + dir + "/binary\n" + dir + "/empty\n" + dir + "/a.txt\n" + dir + "/SKILL.md\n",
			), nil
		}
		return []byte("10\n"), nil
	}
	for _, budget := range []string{"11", "10", "9"} {
		t.Setenv("BODY_BUDGET", budget)
		out.Reset()
		m.exceeded = false
		require.NoError(t, m.measure([]string{dir}))
		require.Equal(t, budget != "11", strings.Contains(out.String(), "OVER the"))
		require.Contains(t, out.String(), "bundled     20")
	}
	record, err := os.ReadFile(m.path(dir + "/tokens.json"))
	require.NoError(t, err)
	require.Contains(t, string(record), `"path": "\u00e9\ud83d\ude00"`)
	require.Less(t, strings.Index(string(record), `"path": "a.txt"`), strings.Index(string(record), `"path": "\u00e9`))
	require.Contains(t, stderr.String(), "skip "+dir+"/empty: not text")
	require.Contains(t, stderr.String(), "skip "+dir+"/binary: not text")
}

func TestCompletedRecordSurvivesLaterFailure(t *testing.T) {
	t.Parallel()
	m, _, _, dir := fixture(t)
	first := m.run
	calls := 0
	m.run = func(name string, args []string, input string) ([]byte, error) {
		calls++
		if calls > 5 {
			return nil, io.ErrUnexpectedEOF
		}
		return first(name, args, input)
	}
	require.Error(t, m.measure([]string{dir, dir}))
	record, err := os.ReadFile(m.path(dir + "/tokens.json"))
	require.NoError(t, err)
	require.Contains(t, string(record), `"skill": "example"`)
}

func TestCountAndRecordErrors(t *testing.T) {
	n, countErr := countValue([]byte("12\r\n"))
	require.NoError(t, countErr)
	require.Equal(t, int64(12), n)
	m, _, _, _ := fixture(t)
	t.Setenv("BODY_BUDGET", "invalid")
	require.Error(t, m.measure(nil))
	for _, value := range []string{"", "-1", "1.2", "true", "10\n10", " 1"} {
		_, err := countValue([]byte(value))
		require.Error(t, err)
	}
	_, err := recordBytes(map[string]any{"unsupported": make(chan int)})
	require.Error(t, err)
	require.Equal(t, "a\nb\nc", text([]byte("a\r\nb\rc")))
}

func TestMeasurementAndTotalFailures(t *testing.T) {
	t.Parallel()
	t.Run("read fails after discovery", func(t *testing.T) {
		t.Parallel()
		m, _, _, dir := fixture(t)
		first := m.run
		m.run = func(name string, args []string, input string) ([]byte, error) {
			if name == "git" {
				return []byte(dir + "/missing\n"), nil
			}
			return first(name, args, input)
		}
		require.Error(t, m.measure([]string{dir}))
		_, err := os.Stat(m.path(dir + "/tokens.json"))
		require.True(t, os.IsNotExist(err))
	})
	t.Run("total overflow", func(t *testing.T) {
		t.Parallel()
		m, _, _, dir := fixture(t)
		for _, name := range []string{"a", "b"} {
			require.NoError(t, os.WriteFile(m.path(dir+"/"+name), []byte("text"), 0o600))
		}
		m.run = func(name string, _ []string, _ string) ([]byte, error) {
			if name == "git" {
				return []byte(dir + "/a\n" + dir + "/b\n"), nil
			}
			return []byte("9223372036854775807\n"), nil
		}
		require.ErrorContains(t, m.measure([]string{dir}), "overflow")
		_, err := os.Stat(m.path(dir + "/tokens.json"))
		require.True(t, os.IsNotExist(err))
	})
}

func failingProducer(failAt int, response, dir string) execute {
	calls := 0
	return func(name string, _ []string, _ string) ([]byte, error) {
		calls++
		if calls == failAt {
			if response == "10\n" || response == "" {
				return []byte(response), io.ErrUnexpectedEOF
			}
			return []byte(response), nil
		}
		if name == "git" {
			return []byte(dir + "/extra\n"), nil
		}
		return []byte("10\n"), nil
	}
}

func TestDefaultDirsExcludesHiddenWildcardComponents(t *testing.T) {
	t.Parallel()
	m, _, _, dir := fixture(t)
	for _, hidden := range []string{
		"packages/agents/codex/.apm/skills/.hidden",
		"packages/agents/.hidden/.apm/skills/example",
	} {
		require.NoError(t, os.MkdirAll(m.path(hidden), 0o750))
	}
	dirs, err := m.defaultDirs()
	require.NoError(t, err)
	require.Equal(t, []string{dir}, dirs)
}

func TestUnicodeSkillSummaryUsesByteWidth(t *testing.T) {
	t.Parallel()
	m, out, _, dir := fixture(t)
	unicodeDir := filepath.ToSlash(filepath.Join(filepath.Dir(dir), "café"))
	require.NoError(t, os.Rename(m.path(dir), m.path(unicodeDir)))
	require.NoError(t, m.measure([]string{unicodeDir}))
	require.Contains(t, out.String(), "café                    description")
	require.NotContains(t, out.String(), "café                     description")
}
