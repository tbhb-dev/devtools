// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package tokens measures skill context costs with the existing counting backends.
package tokens

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

var (
	errCount    = errors.New("invalid token count")
	errUTF8     = errors.New("invalid UTF-8")
	errOverflow = errors.New("bundled token total overflow")
)

const (
	summaryWidth = 24
	asciiLimit   = 127
	maxBasicRune = 0xffff
)

type bundledRow struct {
	Bytes  int    `json:"bytes"`
	Path   string `json:"path"`
	Tokens int64  `json:"tokens"`
}

type execute func(name string, args []string, input string) ([]byte, error)

type measurement struct {
	root                    string
	run                     execute
	out, stderr             io.Writer
	claude, codex, encoding string
	baseline                *int64
	budget                  int64
	exceeded                bool
}

func setting(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Run measures explicit directories, or all packaged skills when none are given.
func Run(ctx context.Context, dirs []string, out, stderr io.Writer) int {
	root, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		fmt.Fprintf(stderr, "skill-tokens: %v\n", err)
		return 1
	}
	m := measurement{
		root: strings.TrimRight(string(root), "\r\n"), out: out, stderr: stderr,
		claude: setting("CLAUDE_MODEL", setting("MODEL", "claude-opus-5")),
		codex:  setting("CODEX_MODEL", "gpt-5.6-sol"), encoding: setting("CODEX_ENCODING", "o200k_base"),
	}
	m.run = func(name string, args []string, input string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = m.root
		// A regular file preserves ant's stdin detection contract.
		f, e := os.CreateTemp("", "skill-tokens-*")
		if e != nil {
			return nil, fmt.Errorf("request input: %w", e)
		}
		defer os.Remove(f.Name())
		defer f.Close()
		if _, e = f.WriteString(input); e != nil {
			return nil, fmt.Errorf("request input: %w", e)
		}
		if _, e = f.Seek(0, io.SeekStart); e != nil {
			return nil, fmt.Errorf("request input: %w", e)
		}
		cmd.Stdin = f
		cmd.Env = append(
			os.Environ(),
			"LC_ALL=C",
			"PYTHONUTF8=1",
			"CLAUDE_MODEL="+m.claude,
			"CODEX_MODEL="+m.codex,
			"CODEX_ENCODING="+m.encoding,
		)
		if name != "ant" {
			cmd.Stderr = stderr
		}
		data, e := cmd.Output()
		if e != nil {
			return nil, fmt.Errorf("%s: %w", name, e)
		}
		return data, nil
	}
	if err = m.measure(dirs); err != nil {
		fmt.Fprintf(stderr, "skill-tokens: %v\n", err)
		return 1
	}
	return 0
}

func text(data []byte) string {
	return strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
}

func countValue(data []byte) (int64, error) {
	s := strings.TrimRight(string(data), "\r\n")
	if s == "" || strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, fmt.Errorf("%w: %q", errCount, s)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("token count: %w", err)
	}
	return n, nil
}

func (m *measurement) raw(input string, codex bool) (int64, error) {
	var data []byte
	var err error
	if codex {
		data, err = m.run(
			"uv",
			[]string{
				"run",
				"--quiet",
				"--with",
				"tiktoken==0.13.0",
				"python",
				"-c",
				"import sys, tiktoken\nencoding = tiktoken.get_encoding(sys.argv[1])\nprint(len(encoding.encode(sys.stdin.read())))\n",
				m.encoding,
			},
			input,
		)
	} else {
		var request strings.Builder
		request.WriteString("model: " + m.claude + "\nmessages:\n  - role: user\n    content: |\n")
		for line := range strings.SplitSeq(input, "\n") {
			request.WriteString("      " + line + "\n")
		}
		data, err = m.run("ant", []string{"messages", "count-tokens"}, request.String())
		if err == nil {
			var response map[string]json.RawMessage
			if err = json.Unmarshal(data, &response); err == nil {
				data = response["input_tokens"]
			}
		}
	}
	if err != nil {
		return 0, fmt.Errorf("count backend: %w", err)
	}
	return countValue(data)
}

func (m *measurement) count(input string, codex bool) (int64, error) {
	n, err := m.raw(input, codex)
	if err != nil {
		return 0, err
	}
	if !codex {
		n -= *m.baseline
	}
	if n < 0 {
		return 0, fmt.Errorf("%w: %d", errCount, n)
	}
	return n, nil
}

var (
	manifestParts     = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)$`)
	foldedDescription = regexp.MustCompile(`(?m)^description: >-\n((?:  .*\n?)+)`)
)

func partition(s string) (string, string, string) {
	parts := manifestParts.FindStringSubmatch(s)
	if parts == nil {
		return "", "", s
	}
	description := ""
	if d := foldedDescription.FindStringSubmatch(parts[1]); d != nil {
		description = strings.Join(strings.FieldsFunc(d[1], pythonSpace), " ")
	}
	return parts[1], description, parts[2]
}

func isCodex(dir string) bool {
	return strings.HasPrefix(dir, "packages/agents/codex/") || strings.Contains(dir, "/packages/agents/codex/") ||
		strings.HasPrefix(dir, ".agents/skills/") ||
		strings.Contains(dir, "/.agents/skills/")
}

func (m *measurement) path(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(m.root, filepath.FromSlash(p))
}

func (m *measurement) measure(dirs []string) error {
	var err error
	m.budget, err = strconv.ParseInt(setting("BODY_BUDGET", "5000"), 10, 64)
	if err != nil {
		return fmt.Errorf("measure: %w", err)
	}
	if len(dirs) == 0 {
		dirs, err = m.defaultDirs()
		if err != nil {
			return err
		}
	}
	for _, dir := range dirs {
		if err = m.skill(strings.TrimSuffix(filepath.ToSlash(dir), "/")); err != nil {
			return fmt.Errorf("measure: %w", err)
		}
	}
	fmt.Fprint(
		m.out,
		"\nDescriptions load every turn. Manifests load on invocation.\nBundled files cost nothing until something reads them.\n",
	)
	if m.exceeded {
		fmt.Fprintf(
			m.out,
			"\nAt least one body sits past the %d-token budget. Move reference\nmaterial into a bundled file, which costs nothing until read.\n",
			m.budget,
		)
	}
	return nil
}

func (m *measurement) skill(dir string) error {
	manifest := m.path(dir + "/SKILL.md")
	if !regularFile(manifest) {
		fmt.Fprintf(m.stderr, "skip %s: no SKILL.md\n", dir)
		return nil
	}
	codex := isCodex(dir)
	model, err := m.prepare(codex)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		return fmt.Errorf("measure: %w", err)
	}
	if !utf8.Valid(data) {
		return fmt.Errorf("%w in %s", errUTF8, manifest)
	}
	fm, desc, body := partition(text(data))
	var counts [4]int64
	for i, input := range []string{text(data), fm, desc, body} {
		counts[i], err = m.count(input, codex)
		if err != nil {
			return fmt.Errorf("measure: %w", err)
		}
	}
	rows, bundled, err := m.bundled(dir, codex)
	if err != nil {
		return err
	}
	doc := map[string]any{
		"skill":        filepath.Base(dir),
		"model":        model,
		"generated_by": "tools/skill-tokens.sh",
		"manifest": map[string]any{
			"tokens":             counts[0],
			"bytes":              len(data),
			"frontmatter_tokens": counts[1],
			"body_tokens":        counts[3],
		},
		"always_loaded": map[string]any{
			"description_tokens": counts[2],
		},
		"bundled_files":        rows,
		"bundled_files_tokens": bundled,
	}
	if codex {
		doc["tokenizer"] = m.encoding
	}
	record, err := recordBytes(doc)
	if err != nil {
		return fmt.Errorf("measure: %w", err)
	}
	if err = os.WriteFile(m.path(dir+"/tokens.json"), record, 0o600); err != nil {
		return fmt.Errorf("measure: %w", err)
	}
	over := ""
	if counts[3] >= m.budget {
		over = fmt.Sprintf("  OVER the %d-token body budget", m.budget)
		m.exceeded = true
	}
	name := filepath.Base(dir)
	padding := strings.Repeat(" ", max(0, summaryWidth-len(name)))
	fmt.Fprintf(
		m.out,
		"%s%s description %5d   manifest %6d   bundled %6d%s\n",
		name,
		padding,
		counts[2],
		counts[0],
		bundled,
		over,
	)
	return nil
}

// Match Python json.dump's sorted, escaped, indented record bytes.
func recordBytes(doc map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode record: %w", err)
	}
	var ascii strings.Builder
	for _, r := range buf.String() {
		switch {
		case r < asciiLimit:
			ascii.WriteRune(r)
		case r <= maxBasicRune:
			fmt.Fprintf(&ascii, `\u%04x`, r)
		default:
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&ascii, `\u%04x\u%04x`, hi, lo)
		}
	}
	return []byte(ascii.String()), nil
}

func (m *measurement) prepare(codex bool) (string, error) {
	if codex {
		return m.codex, nil
	}
	if m.baseline == nil {
		n, err := m.raw("x", false)
		if err != nil {
			return "", fmt.Errorf("the ant call failed. Check what ant auth status reports: %w", err)
		}
		n--
		m.baseline = &n
		fmt.Fprintf(m.out, "model %s, request envelope %d tokens\n\n", m.claude, n)
	}
	return m.claude, nil
}

func (m *measurement) bundled(dir string, codex bool) ([]bundledRow, int64, error) {
	files, err := m.run(
		"git",
		[]string{"-c", "core.quotePath=false", "ls-files", "--cached", "--others", "--exclude-standard", "--", dir},
		"",
	)
	if err != nil {
		return nil, 0, fmt.Errorf("measure: %w", err)
	}
	paths := strings.Split(strings.TrimSuffix(string(files), "\n"), "\n")
	sort.Strings(paths)
	rows := []bundledRow{}
	var bundled int64
	for _, file := range paths {
		if file == "" || strings.HasSuffix(file, "/SKILL.md") || strings.HasSuffix(file, "/tokens.json") {
			continue
		}
		row, include, e := m.bundledFile(dir, file, codex)
		if e != nil {
			return nil, 0, e
		}
		if !include {
			continue
		}
		n := row.Tokens
		if n > math.MaxInt64-bundled {
			return nil, 0, errOverflow
		}
		bundled += n
		rows = append(rows, row)
	}
	return rows, bundled, nil
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (m *measurement) bundledFile(dir, file string, codex bool) (bundledRow, bool, error) {
	content, err := os.ReadFile(m.path(file))
	if err != nil {
		return bundledRow{}, false, fmt.Errorf("bundled read: %w", err)
	}
	if bytes.IndexByte(content, 0) >= 0 || len(bytes.Trim(content, "\n")) == 0 {
		fmt.Fprintf(m.stderr, "skip %s: not text, so no reader ever pays for it\n", file)
		return bundledRow{}, false, nil
	}
	if !utf8.Valid(content) {
		return bundledRow{}, false, fmt.Errorf("%w in %s", errUTF8, file)
	}
	n, err := m.count(text(content), codex)
	if err != nil {
		return bundledRow{}, false, err
	}
	relative, err := filepath.Rel(m.path(dir), m.path(file))
	if err != nil {
		return bundledRow{}, false, fmt.Errorf("relative bundled path: %w", err)
	}
	return bundledRow{Path: filepath.ToSlash(relative), Tokens: n, Bytes: len(content)}, true, nil
}

func (m *measurement) defaultDirs() ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(m.root, "packages/agents/*/.apm/skills/*"))
	if err != nil {
		return nil, fmt.Errorf("skill discovery: %w", err)
	}
	var dirs []string
	for _, match := range matches {
		info, statErr := os.Stat(match)
		if statErr != nil || !info.IsDir() {
			continue
		}
		relative, relErr := filepath.Rel(m.root, match)
		if relErr != nil {
			return nil, fmt.Errorf("relative skill path: %w", relErr)
		}
		// Bash wildcards exclude hidden names. The literal .apm still matches.
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if strings.HasPrefix(parts[2], ".") || strings.HasPrefix(parts[5], ".") {
			continue
		}
		dirs = append(dirs, filepath.ToSlash(relative))
	}
	if len(dirs) == 0 {
		dirs = []string{"packages/agents/*/.apm/skills/*"}
	}
	return dirs, nil
}

// Python's whitespace class also includes the information separators.
func pythonSpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= '\x1c' && r <= '\x1f')
}
