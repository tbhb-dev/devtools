// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package commitlint_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/tbhb-dev/devtools/internal/commitlint"
)

type goldenCase struct {
	Env     map[string]string `json:"env"`
	Name    string            `json:"name"`
	Rules   string            `json:"rules"`
	Message []string          `json:"message"`
	Status  int               `json:"status"`
	Output  string            `json:"output"`
}

func TestLegacyGolden(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/legacy.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []goldenCase
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		t.Run(tt.Rules+"/"+tt.Name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			status := commitlint.Run(
				[]string{"--rules", tt.Rules},
				strings.NewReader(strings.Join(tt.Message, "\n")),
				&output,
				t.TempDir(),
				func(key string) string { return tt.Env[key] },
			)
			if status != tt.Status || output.String() != tt.Output {
				t.Fatalf("got (%d, %q), want (%d, %q)", status, output.String(), tt.Status, tt.Output)
			}
		})
	}
}

func TestParse(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, raw string
		lines     []string
		footer    int
	}{
		{"empty", "", []string{""}, -1},
		{"cleanup", "\n# comment\n\nfix: a thing  \n\n\nbody\t\n\n", []string{"fix: a thing", "", "body"}, -1},
		{"semicolon scissors", "fix: a thing\n; --- >8 ---\ndiff", []string{"fix: a thing"}, -1},
		{"footer only", "fix: a thing\n\nSigned-off-by: T", []string{"fix: a thing", "", "Signed-off-by: T"}, 2},
		{"folded footer", "fix: a thing\n\nbody\n\nBREAKING CHANGE: value\n continuation", []string{"fix: a thing", "", "body", "", "BREAKING CHANGE: value", " continuation"}, 4},
		{"no footer boundary", "fix: a thing\n\nbody\nToken: value", []string{"fix: a thing", "", "body", "Token: value"}, -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := commitlint.Parse(tt.raw)
			if !reflect.DeepEqual(m.Lines, tt.lines) || m.FooterStart != tt.footer {
				t.Fatalf("got %#v", m)
			}
		})
	}
}

func TestAdditionalRules(t *testing.T) {
	t.Parallel()
	signoff := "\n\nSigned-off-by: T"
	for _, tt := range []struct {
		name, message, rules string
		config               commitlint.Config
		want                 string
	}{
		{"unicode header", "fix: café" + signoff, "shape", commitlint.DefaultConfig(), "commitlint: subject is too short (9 < 10): \"fix: café\""},
		{"footer continuation cap", "fix: a thing\n\nToken: value\n " + strings.Repeat("x", 100) + "\nSigned-off-by: T", "shape", commitlint.DefaultConfig(), "commitlint: footer line 4 exceeds 100 chars (101)"},
		{"raw trailer compatibility", "fix: a thing\n\nSigned-off-by:\n# --- >8 ---\nAssisted-by: bad/value", "trailers", commitlint.DefaultConfig(), "commit-trailers: malformed Assisted-by value: expected AGENT_NAME:MODEL_VERSION [TOOL...] (line 5: \"bad/value\")\ncommit-trailers: trailer order: Assisted-by must appear before Signed-off-by (line 5 after line 3)"},
		{"duplicate trailers use first occurrence", "fix: a thing\n\nAssisted-by: codex:gpt-6\nSigned-off-by: T\nAssisted-by: codex:gpt-6", "trailers", commitlint.DefaultConfig(), ""},
		{"empty signoff differs by rule", "fix: a thing\n\nSigned-off-by:", "shape", commitlint.DefaultConfig(), "commitlint: missing required Signed-off-by trailer (DCO)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := strings.Join(
				commitlint.CheckWithIdentities(commitlint.Parse(tt.message), tt.config, tt.rules, nil),
				"\n",
			)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
	c := commitlint.DefaultConfig()
	c.RequireSignedOffBy = false
	if got := commitlint.CheckWithIdentities(commitlint.Parse("feat: a change"), c, "all", nil); len(got) != 0 {
		t.Fatal(got)
	}
	if got := commitlint.CheckWithIdentities(
		commitlint.Parse("invalid"),
		commitlint.DefaultConfig(),
		"all",
		nil,
	); len(got) != 4 {
		t.Fatal(got)
	}
}

func TestAgentCommitRequiresWellFormedAssistedBy(t *testing.T) {
	t.Parallel()
	config := commitlint.DefaultConfig()
	config.Identities.Agents = []commitlint.Identity{{Email: "agent@example.com"}}
	identity := []string{"Agent <agent@example.com>"}

	without := strings.Join(commitlint.CheckWithIdentities(
		commitlint.Parse("fix: attribute the change\n\nSigned-off-by: Person <person@example.com>"),
		config,
		"trailers",
		identity,
	), "\n")
	if !strings.Contains(without, "configured agent author or committer requires a well-formed Assisted-by trailer") {
		t.Fatalf("missing agent-attribution finding: %q", without)
	}

	with := commitlint.CheckWithIdentities(
		commitlint.Parse(
			"fix: attribute the change\n\n"+
				"Assisted-by: codex:gpt-6\n"+
				"Signed-off-by: Person <person@example.com>",
		),
		config,
		"trailers",
		identity,
	)
	if len(with) != 0 {
		t.Fatalf("well-formed attribution failed: %q", with)
	}
}

func FuzzParse(f *testing.F) {
	for _, seed := range []string{"", "fix: a thing\n\nSigned-off-by: T", "# comment\n\nfeat(api)!: change\n\nToken: value\n folded", "; --- >8 ---\ndiff", "\xff\x00\r\n", "; --- >8 ---\t"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		m := commitlint.Parse(raw)
		if len(m.Lines) == 0 || m.FooterStart >= len(m.Lines) || m.FooterStart < -1 {
			t.Fatalf("invalid parse: %#v", m)
		}
		for i, line := range m.Lines {
			if line != strings.TrimRightFunc(line, unicode.IsSpace) || (i > 0 && line == "" && m.Lines[i-1] == "") {
				t.Fatalf("unclean lines: %#v", m.Lines)
			}
		}
		commitlint.CheckWithIdentities(m, commitlint.DefaultConfig(), "all", nil)
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "message")
	writeFile(t, file, "fix: a thing\n\nSigned-off-by: T")
	missing := filepath.Join(dir, "missing")
	_, missingErr := os.ReadFile(missing)
	if !errors.Is(missingErr, os.ErrNotExist) {
		t.Fatalf("expected a missing-file error, got %v", missingErr)
	}
	for _, tt := range []struct {
		name   string
		args   []string
		status int
		want   string
	}{
		{"file", []string{file}, 0, ""},
		{"help", []string{"--help"}, 0, "Usage of commit-lint:"},
		{"unknown flag", []string{"--bogus"}, 2, "flag provided but not defined"},
		{"bad rules", []string{"--rules", "bogus"}, 2, "expected --rules"},
		{"extra file", []string{file, file}, 2, "expected --rules"},
		{"missing file", []string{missing}, 2, "commit-lint: " + missingErr.Error() + "\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			status := commitlint.Run(tt.args, strings.NewReader(""), &output, dir, func(string) string { return "" })
			if status != tt.status || !strings.Contains(output.String(), tt.want) {
				t.Fatalf("got %d, %q", status, output.String())
			}
		})
	}
	writeFile(t, filepath.Join(dir, ".commit-lint.toml"), "invalid = [")
	var output bytes.Buffer
	if status := commitlint.Run(
		nil,
		strings.NewReader(""),
		&output,
		dir,
		func(string) string { return "" },
	); status != 2 ||
		!strings.Contains(output.String(), ".commit-lint.toml") {
		t.Fatalf("got %d, %s", status, &output)
	}
	output.Reset()
	if err := os.Remove(filepath.Join(dir, ".commit-lint.toml")); err != nil {
		t.Fatal(err)
	}
	if status := commitlint.Run(
		nil,
		failingReader{},
		&output,
		dir,
		func(string) string { return "" },
	); status != 2 ||
		!strings.Contains(output.String(), "commit-lint: "+os.ErrPermission.Error()+"\n") {
		t.Fatalf("got %d, %s", status, &output)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, os.ErrPermission }
