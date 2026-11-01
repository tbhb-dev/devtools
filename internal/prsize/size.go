// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package prsize measures changed code and prose from a merge base.
package prsize

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Limits holds thresholds. Values exceeding them change the level.
type Limits struct {
	Code  int `json:"code"`
	Prose int `json:"prose"`
}

// Excluded records raw added plus removed lines for an excluded path.
type Excluded struct {
	Path string `json:"path"`
	Raw  int    `json:"raw"`
}

// Result is the shared size report. Levels never change the exit status.
type Result struct {
	Code      int            `json:"code"`
	Prose     int            `json:"prose"`
	Languages map[string]int `json:"languages"`
	Excluded  []Excluded     `json:"excluded"`
	Soft      Limits         `json:"soft"`
	Hard      Limits         `json:"hard"`
	Level     string         `json:"level"`
}

var exclusions = regexp.MustCompile(
	`(^|/)(go\.sum|uv\.lock|Cargo\.lock|package-lock\.json|pnpm-lock\.yaml|yarn\.lock|mise\.lock|apm\.lock\.yaml|[^/]*\.lock|[^/]*\.golden|CHANGELOG\.md)$|(^|/)(vendor|testdata|fixtures|__snapshots__)/|(^|/)skills/[^/]+/tokens\.json$|^packages/agents/(claude|codex)/\.apm/skills/|^\.(claude|agents)/skills/`,
)

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

// Measure compares head to its merge base with base. Empty refs select origin's
// default branch and the head commit. dir may be empty to use the current directory.
func Measure(ctx context.Context, dir, base, head string) (Result, error) {
	r := Result{Languages: map[string]int{}, Excluded: []Excluded{}, Soft: Limits{400, 1000}, Hard: Limits{800, 2000}}
	scc, err := exec.LookPath("scc")
	if err != nil {
		return r, fmt.Errorf(
			"scc is required; install mise pin aqua:boyter/scc in .config/mise/conf.d/10-devtools-tools.toml: %w",
			err,
		)
	}
	if base == "" {
		base, err = git(ctx, dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
		if err != nil {
			return r, err
		}
	}
	if head == "" {
		head = "HEAD"
	}
	// Resolve user refs before passing them to diff so they can't become options.
	base, err = git(ctx, dir, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
	if err != nil {
		return r, err
	}
	head, err = git(ctx, dir, "rev-parse", "--verify", "--end-of-options", head+"^{commit}")
	if err != nil {
		return r, err
	}
	base, err = git(ctx, dir, "merge-base", base, head)
	if err != nil {
		return r, err
	}
	paths, err := git(ctx, dir, "diff", "--name-status", "-z", "--find-renames", base, head, "--")
	if err != nil {
		return r, err
	}
	origin, originErr := git(ctx, dir, "remote", "get-url", "origin")
	devtools := originErr == nil && strings.HasSuffix(strings.TrimSuffix(origin, ".git"), "tbhb-dev/devtools")
	tmp, err := os.MkdirTemp("", "prsize-*")
	if err != nil {
		return r, fmt.Errorf("size temporary directory: %w", err)
	}
	defer os.RemoveAll(tmp)
	if err = r.walk(ctx, dir, base, head, paths, tmp, devtools); err != nil {
		return r, err
	}
	cmd := exec.CommandContext(
		ctx,
		scc,
		"--no-config",
		"--no-gitignore",
		"--no-ignore",
		"--no-scc-ignore",
		"--format",
		"json",
		tmp,
	)
	cmd.Dir = tmp
	out, err := cmd.Output()
	if err != nil {
		return r, fmt.Errorf("scc: %w", err)
	}
	var languages []struct {
		Name string `json:"Name"`
		Code int    `json:"Code"`
	}
	if err = json.Unmarshal(out, &languages); err != nil {
		return r, fmt.Errorf("scc JSON: %w", err)
	}
	for _, language := range languages {
		r.Languages[language.Name] += language.Code
		r.Code += language.Code
	}
	r.setLevel()
	return r, nil
}

func (r *Result) walk(ctx context.Context, dir, base, head, paths, tmp string, devtools bool) error {
	parts := strings.Split(strings.TrimSuffix(paths, "\x00"), "\x00")
	for i := 0; i+1 < len(parts); {
		status, path := parts[i], parts[i+1]
		i += 2
		files := []string{path}
		if strings.HasPrefix(status, "R") {
			path = parts[i]
			files = append(files, path)
			i++
		}
		args := append(
			[]string{
				"diff",
				"--no-color",
				"--no-ext-diff",
				"--no-textconv",
				"--find-renames",
				"--unified=0",
				base,
				head,
				"--",
			},
			files...,
		)
		patch, err := git(ctx, dir, args...)
		if err != nil {
			return err
		}
		added, removed := fragments(patch)
		if exclusions.MatchString(path) || (!devtools && strings.HasPrefix(path, ".devtools/")) {
			r.Excluded = append(r.Excluded, Excluded{path, len(added) + len(removed)})
			continue
		}
		if err = r.addFragments(path, tmp, i, added, removed); err != nil {
			return err
		}
	}
	return nil
}

func (r *Result) addFragments(path, tmp string, index int, fragments ...[]string) error {
	for side, lines := range fragments {
		if prose(path) {
			r.Prose += nonblank(lines)
			continue
		}
		if len(lines) == 0 {
			continue
		}
		folder := filepath.Join(tmp, fmt.Sprintf("%d-%d", index, side))
		if err := os.Mkdir(folder, 0o700); err != nil {
			return fmt.Errorf("fragment directory: %w", err)
		}
		if err := os.WriteFile(
			filepath.Join(folder, filepath.Base(path)),
			[]byte(strings.Join(lines, "\n")+"\n"),
			0o600,
		); err != nil {
			return fmt.Errorf("fragment: %w", err)
		}
	}

	return nil
}

func fragments(patch string) ([]string, []string) {
	var added, removed []string
	inHunk := false
	for line := range strings.SplitSeq(patch, "\n") {
		if strings.HasPrefix(line, "@@ ") {
			inHunk = true
			continue
		}
		if !inHunk || line == "" {
			continue
		}
		switch line[0] {
		case '+':
			added = append(added, line[1:])
		case '-':
			removed = append(removed, line[1:])
		}
	}
	return added, removed
}

func prose(path string) bool {
	switch filepath.Ext(path) {
	case ".md", ".mdx", ".txt", ".rst":
		return true
	default:
		return false
	}
}

func (r *Result) setLevel() {
	r.Level = "ok"
	if r.Code > r.Soft.Code || r.Prose > r.Soft.Prose {
		r.Level = "soft"
	}
	if r.Code > r.Hard.Code || r.Prose > r.Hard.Prose {
		r.Level = "hard"
	}
}

func nonblank(lines []string) int {
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
