// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package pins updates consumer release pins.
package pins

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const guardTool = "guard-markdown"

type updater struct{ dir string }

func (u *updater) exists(file string) bool {
	info, err := os.Stat(filepath.Join(u.dir, file))
	return err == nil && info.Mode().IsRegular()
}

// edit retains the shell's temporary filename and individual replacement order.
// A later failure deliberately leaves earlier replacements in place.
func (u *updater) edit(file string, transform func(string) string) error {
	path := filepath.Join(u.dir, file)
	tmp, err := os.Create(path + ".pins.tmp")
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%w", err)
	}
	_, err = io.WriteString(tmp, transform(string(body)))
	closed := tmp.Close()
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	if closed != nil {
		return fmt.Errorf("%w", closed)
	}
	if err = os.Rename(path+".pins.tmp", path); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func substitute(pattern, replacement string) func(string) string {
	re := regexp.MustCompile(pattern)
	return func(body string) string {
		// sed replaces only the first match on each line.
		lines := strings.Split(body, "\n")
		for i, line := range lines {
			if loc := re.FindStringSubmatchIndex(line); loc != nil {
				lines[i] = line[:loc[0]] + string(re.ExpandString(nil, replacement, line, loc)) + line[loc[1]:]
			}
		}
		return strings.Join(lines, "\n")
	}
}

type replacement struct {
	file      string
	transform func(string) string
}

func (u *updater) rewrite(version, sha string) error {
	edits := []replacement{
		{
			"apm.yml",
			substitute(
				`(tbhb-dev/devtools(/packages/agents/(common|claude|codex|agy))?#)v[^[:space:]"']+`,
				"${1}"+version,
			),
		},
		{"tombi.toml", func(body string) string {
			return newline(
				substitute(
					`^(.*tbhb-dev/devtools[[:blank:]]+)v[0-9]+\.[0-9]+\.[0-9]+(.*)$`,
					"${1}"+version+"${2}",
				)(
					body,
				),
			)
		}},
	}
	for _, tool := range []string{guardTool, "commit-lint", "gh-post", "agentrepo"} {
		pattern := `^("go:github.com/tbhb-dev/devtools/cmd/` + tool + `"[[:blank:]]*=[[:blank:]]*)"[^"]*"`
		edits = append(edits, replacement{"mise.toml", substitute(pattern, `${1}"`+version+`"`)})
	}
	workflows, err := u.workflowEdits(version, sha)
	if err != nil {
		return err
	}
	edits = append(edits, workflows...)
	edits = append(
		edits,
		replacement{
			".github/renovate.json5",
			substitute(`(github>tbhb-dev/devtools//\.github/[^/"]+\.json5)(#[^"]+)"`, `${1}#`+sha+`"`),
		},
		replacement{
			".github/renovate.json5",
			substitute(
				`(github>tbhb-dev/devtools//\.github/[^/"]+\.json5#[0-9a-f]{7,40}",?[[:blank:]]*//[[:blank:]]*)v[^[:space:]]*`,
				"${1}"+version,
			),
		},
		replacement{"vendir.yml", block("url", "ref", version)},
		replacement{".pre-commit-config.yaml", block("repo", "rev", sha+"  # frozen: "+version)},
	)
	for _, edit := range edits {
		if edit.file != "mise.toml" && !u.exists(edit.file) {
			continue
		}
		if err = u.edit(edit.file, edit.transform); err != nil {
			return err
		}
	}
	return nil
}

func (u *updater) workflowEdits(version, sha string) ([]replacement, error) {
	var edits []replacement
	for _, ext := range []string{"yml", "yaml"} {
		files, err := filepath.Glob(filepath.Join(u.dir, ".github/workflows/*."+ext))
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		for _, path := range files {
			file, relErr := filepath.Rel(u.dirOrDot(), path)
			if relErr != nil {
				return nil, fmt.Errorf("%w", relErr)
			}
			pattern := `(uses:[[:blank:]]*tbhb-dev/devtools/[^@]*@)[0-9a-f]{7,40}([[:blank:]]*#[[:blank:]]*)v[^[:space:]]*`
			edits = append(edits, replacement{file, substitute(pattern, "${1}"+sha+"${2}"+version)})
		}
	}
	return edits, nil
}

func block(key, field, value string) func(string) string {
	repo := regexp.MustCompile(key + `:[[:blank:]]*https://github\.com/tbhb-dev/devtools([[:space:]]|$)`)
	target := regexp.MustCompile(`^([[:blank:]]*)` + field + `:[[:blank:]]*`)
	return func(body string) string {
		if body == "" {
			return ""
		}
		lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
		active := false
		for i, line := range lines {
			if repo.MatchString(line) {
				active = true
			}
			if active {
				if match := target.FindStringSubmatch(line); match != nil {
					lines[i] = match[1] + field + ": " + value
					active = false
				}
			}
		}
		return strings.Join(lines, "\n") + "\n"
	}
}

func newline(body string) string {
	if body != "" && !strings.HasSuffix(body, "\n") {
		return body + "\n"
	}
	return body
}

func (u *updater) dirOrDot() string {
	if u.dir == "" {
		return "."
	}
	return u.dir
}
