// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package versions checks the repository's published release literals.
package versions

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type checker struct {
	root     string
	out      io.Writer
	findings int
}

// Run checks the Git working tree at dir. An empty version selects the nearest tag.
func Run(ctx context.Context, dir, version string, out, stderr io.Writer) int {
	const apmShape = "^version: "
	const readme = "README.md"
	root, err := gitRead(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "check-versions: %v\n", err)
		return 1
	}
	if version == "" {
		version, err = gitRead(ctx, dir, "describe", "--tags", "--abbrev=0")
		if err != nil {
			_, _ = fmt.Fprintln(
				stderr,
				"check-versions: no tag reachable from HEAD; fetch tags and unshallow before running this",
			)
			return 1
		}
	}
	version = strings.TrimPrefix(version, "v")
	c := checker{root: root, out: out}
	c.apmLock()
	for _, site := range []struct{ file, shape, want string }{
		{"apm.yml", apmShape, "version: " + version},
		{"packages/agents/common/apm.yml", apmShape, "version: " + version},
		{"packages/agents/claude/apm.yml", apmShape, "version: " + version},
		{"packages/agents/codex/apm.yml", apmShape, "version: " + version},
		{"packages/agents/agy/apm.yml", apmShape, "version: " + version},
		{"packages/devtools/pyproject.toml", "^version = ", `version = "` + version + `"`},
		{".pre-commit-hooks.yaml", "rev: v[0-9]", "rev: v" + version},
		{readme, "tbhb-dev/devtools#v[0-9]", "tbhb-dev/devtools#v" + version},
		{readme, "tbhb-dev/devtools/packages", "#v" + version},
		{readme, "rev: v[0-9]", "rev: v" + version},
	} {
		c.site(site.file, site.shape, site.want)
	}
	c.uvLock(`version = "` + version + `"`)
	if c.findings > 0 {
		_, _ = fmt.Fprintf(out, "TOTAL: %d finding(s) against v%s\n", c.findings, version)
		return 1
	}
	return 0
}

func gitRead(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	data, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (c *checker) report(file string, line int, message string) {
	_, _ = fmt.Fprintf(c.out, "%s:%d [error] version-literal-stale  %s\n", file, line, message)
	c.findings++
}

func (c *checker) read(file string) []string {
	data, err := os.ReadFile(filepath.Join(c.root, file))
	if err != nil {
		c.report(file, 1, "cannot read file")
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func (c *checker) compare(file string, line int, text, want string) {
	if !strings.Contains(text, want) {
		c.report(file, line, fmt.Sprintf("expected `%s`, found `%s`", want, text))
	}
}

func (c *checker) site(file, shape, want string) {
	lines := c.read(file)
	if lines == nil {
		return
	}
	pattern := regexp.MustCompile(shape)
	found := false
	for i, line := range lines {
		if pattern.MatchString(line) {
			found = true
			c.compare(file, i+1, line, want)
		}
	}
	if !found {
		c.report(
			file,
			1,
			"no line matching /"+shape+"/; the site moved or was deleted, and whatever cog.toml hook rewrites it is now inert",
		)
	}
}

func (c *checker) uvLock(want string) {
	const file = "uv.lock"
	lines := c.read(file)
	if lines == nil {
		return
	}
	found := false
	for i, line := range lines {
		if line == `name = "devtools"` && i+1 < len(lines) {
			found = true
			versionIndex := i + 1
			c.compare(file, versionIndex+1, lines[versionIndex], want)
		}
	}
	if !found {
		c.report(
			file,
			1,
			"no package entry named devtools; the lockfile shape changed and the cog.toml hook that rewrites it is now inert",
		)
	}
}

func (c *checker) apmLock() {
	const file = "apm.lock.yaml"
	lines := c.read(file)
	if lines == nil {
		return
	}
	for _, name := range []string{"common", "claude", "codex", "agy"} {
		path := "packages/agents/" + name
		manifest := c.read(path + "/apm.yml")
		version := ""
		for _, line := range manifest {
			if value, ok := strings.CutPrefix(line, "version: "); ok {
				version = value
			}
		}
		c.apmEntry(lines, path, version)
	}
}

func (c *checker) apmEntry(lines []string, path, want string) {
	const file = "apm.lock.yaml"
	found := false
	version, number := "", 0
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "- repo_url:"):
			version, number = "", 0
		case strings.HasPrefix(line, "  version: "):
			version, number = strings.TrimPrefix(line, "  version: "), i+1
		case line == "  local_path: ./"+path:
			found = true
			if version != want {
				c.report(
					file,
					number,
					fmt.Sprintf("%s expected `version: %s`, found `version: %s`", path, want, version),
				)
			}
		}
	}
	if !found {
		c.report(file, 1, "no local package entry for "+path)
	}
}
