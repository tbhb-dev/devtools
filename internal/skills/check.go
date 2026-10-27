// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package skills preserves the offline shell skill-limit verdicts.
package skills

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	nameMax        = 64
	descriptionMax = 1024
)

// Run checks targets relative to the Git root at dir, discovering skills when empty.
func Run(ctx context.Context, dir string, targets []string, out, stderr io.Writer) int {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	data, err := cmd.Output()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "check-skill-limits: %v\n", err)
		return 1
	}
	return Check(strings.TrimRight(string(data), "\r\n"), targets, out)
}

// Check reads manifests under root without changing the working directory or tree.
func Check(root string, targets []string, out io.Writer) int {
	if len(targets) == 0 {
		var err error
		targets, err = discover(root)
		if err != nil {
			_, _ = fmt.Fprintf(out, "check-skill-limits: %v\n", err)
			return 1
		}
	}
	findings := 0
	for _, target := range targets {
		manifest := strings.TrimSuffix(target, "/") + "/SKILL.md"
		path := manifest
		if !filepath.IsAbs(path) {
			// Keep .. and missing components for filesystem resolution, as Bash does.
			path = root + string(filepath.Separator) + path
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, readErr := os.ReadFile(path)
		report := func(rule, message string) {
			_, _ = fmt.Fprintf(out, "%s:1 [error] %s  %s\n", manifest, rule, message)
			findings++
		}
		if readErr != nil || !utf8.Valid(data) {
			report("unreadable", "could not parse the frontmatter")
			continue
		}
		name, description := fields(string(data))
		validate(name, description, report)
	}
	if findings > 0 {
		_, _ = fmt.Fprintf(out, "TOTAL: %d finding(s)\n", findings)
		return 1
	}
	return 0
}

func discover(root string) ([]string, error) {
	tree := os.DirFS(root)
	// Bash wildcards exclude dot-prefixed names at each wildcard component.
	packaged, err := fs.Glob(tree, "packages/agents/[^.]*/.apm/skills/[^.]*")
	if err != nil {
		return nil, fmt.Errorf("packaged skills: %w", err)
	}
	local, err := fs.Glob(tree, ".claude/skills/[^.]*")
	if err != nil {
		return nil, fmt.Errorf("local skills: %w", err)
	}
	names := make(map[string]bool)
	var targets []string
	for _, path := range packaged {
		if info, statErr := fs.Stat(tree, path); statErr == nil && info.IsDir() {
			targets = append(targets, path+"/")
			names[filepath.Base(path)] = true
		}
	}
	for _, path := range local {
		if info, statErr := fs.Stat(tree, path); statErr == nil && info.IsDir() && !names[filepath.Base(path)] {
			targets = append(targets, ".claude/skills/"+filepath.Base(path)+"/")
		}
	}
	return targets, nil
}

func fields(text string) (string, string) {
	// pathlib.read_text uses universal newlines. The shell uses byte lengths
	// after Python extraction, because LC_ALL=C is pinned by the entry point.
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	frontmatter := regexp.MustCompile(`(?s)^---\n(.*?)\n---\n`).FindStringSubmatch(text)
	if frontmatter == nil {
		return "", ""
	}
	fm := frontmatter[1]
	name, _ := capture(`(?m)^name:[ \t]*(.+)$`, fm)
	desc, matched := capture(`(?m)^description: >-\n((?:[ \t]+.*\n)+)`, fm)
	if matched {
		desc = strings.Join(strings.FieldsFunc(desc, pythonSpace), " ")
	} else {
		desc, _ = capture(`(?m)^description:[ \t]*(.+)$`, fm)
	}
	return name, desc
}

func capture(pattern, text string) (string, bool) {
	match := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if match == nil {
		return "", false
	}
	// Bash variables discard null bytes from Python output.
	return strings.ReplaceAll(strings.TrimFunc(match[1], pythonSpace), "\x00", ""), true
}

func pythonSpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= '\x1c' && r <= '\x1f')
}

func validate(name, desc string, report func(string, string)) {
	if name == "" {
		report("name-missing", "the frontmatter sets no name")
	} else {
		if len(name) > nameMax {
			report(
				"name-too-long",
				fmt.Sprintf("the name runs %d characters, past the %d the platform allows", len(name), nameMax),
			)
		}
		if strings.ContainsFunc(
			name,
			func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' },
		) {
			report(
				"name-charset",
				fmt.Sprintf("\"%s\" holds something outside lowercase letters, numbers, and hyphens", name),
			)
		}
		if strings.Contains(name, "anthropic") || strings.Contains(name, "claude") {
			report(
				"name-reserved",
				fmt.Sprintf("\"%s\" carries a reserved word; the platform rejects anthropic and claude", name),
			)
		}
	}
	if desc == "" {
		report("description-missing", "the frontmatter sets no description")
	} else if len(desc) > descriptionMax {
		report(
			"description-too-long",
			fmt.Sprintf(
				"the description runs %d characters, past the %d the platform allows",
				len(desc),
				descriptionMax,
			),
		)
	}
	text := name + desc
	if start := strings.IndexByte(text, '<'); start >= 0 && strings.Contains(text[start+1:], ">") {
		report("xml-tags", "the name or description holds what reads as an XML tag")
	}
}
