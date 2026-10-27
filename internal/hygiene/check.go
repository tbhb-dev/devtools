// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package hygiene checks repository shell scripts without changing them.
package hygiene

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const commandUnavailable = 127

type runner func(context.Context, string, string, ...string) ([]byte, []byte, int)

func command(ctx context.Context, root, name string, args ...string) ([]byte, []byte, int) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err := cmd.Run()
	if err == nil {
		return out.Bytes(), stderr.Bytes(), 0
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return out.Bytes(), stderr.Bytes(), exit.ExitCode()
	}
	fmt.Fprintln(&stderr, err)
	return out.Bytes(), stderr.Bytes(), commandUnavailable
}

// Run checks explicit paths, or discovers the frozen command's default sets.
// An empty root resolves the current repository through Git.
func Run(ctx context.Context, root string, paths []string, out, stderr io.Writer) int {
	if root == "" {
		data, diagnostic, status := command(ctx, "", "git", "rev-parse", "--show-toplevel")
		if status != 0 {
			fmt.Fprint(stderr, string(diagnostic))
			return status
		}
		root = strings.TrimRight(string(data), "\r\n")
	}

	if len(paths) == 0 {
		var status int
		paths, status = discover(ctx, root, command, stderr)
		if status != 0 {
			return status
		}
	}
	scan, count, err := selectFiles(root, paths, out)
	if err != nil {
		fmt.Fprintf(stderr, "check-script-hygiene: %s\n", err)
		return 1
	}
	found, status := structural(ctx, root, scan, command, out, stderr)
	if status != 0 {
		return status
	}
	count += found
	if count > 0 {
		fmt.Fprintf(out, "TOTAL: %d finding(s)\n", count)
		return 1
	}
	return 0
}

func discover(ctx context.Context, root string, run runner, stderr io.Writer) ([]string, int) {
	var files []string
	sets := [][]string{
		{"packages/agents/*/.apm/skills/*/scripts/*.sh", "hooks/*.sh", "scripts/*.sh", "tools/*.sh"},
		{".claude/skills/*/scripts/*.sh"},
	}
	for i, patterns := range sets {
		args := append(
			[]string{"-c", "core.quotePath=false", "ls-files", "--cached", "--others", "--exclude-standard"},
			patterns...,
		)
		data, diagnostic, status := run(ctx, root, "git", args...)
		if status != 0 {
			fmt.Fprint(stderr, string(diagnostic))
			return nil, status
		}
		for path := range strings.SplitSeq(strings.TrimRight(string(data), "\n"), "\n") {
			if path == "" || (i == 1 && packaged(root, path)) {
				continue
			}
			files = append(files, path)
		}
	}
	return files, 0
}

func packaged(root, path string) bool {
	skill, _, _ := strings.Cut(strings.TrimPrefix(path, ".claude/skills/"), "/")
	agents, err := os.ReadDir(filepath.Join(root, "packages/agents"))
	if err != nil {
		return false
	}
	for _, agent := range agents {
		info, statErr := os.Stat(filepath.Join(root, "packages/agents", agent.Name(), ".apm/skills", skill))
		if statErr == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func selectFiles(root string, paths []string, out io.Writer) ([]string, int, error) {
	var scan []string
	count := 0
	for _, path := range paths {
		full := path
		if !filepath.IsAbs(path) {
			// Keep symlink/.. traversal for the filesystem to resolve.
			full = root + string(filepath.Separator) + path
		}
		info, err := os.Stat(full)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, count, fmt.Errorf("read %s: %w", path, err)
		}
		if regexp.MustCompile(`(?m)^# hygiene-exempt:`).Match(data) {
			continue
		}
		scan = append(scan, path)
		count += textual(path, data, out)
	}
	return scan, count, nil
}

// textual preserves the shell predicates rather than interpreting commands.
// Only whole comment lines disappear; quoted command text still counts, and
// a hyphen before a command name prevents a match. A locale assignment can
// carry any value. The unset check accepts GH_REPO anywhere after the initial
// space and doesn't require GH_HOST. Tightening those checks would change
// which existing scripts pass, so structural interpretation stays confined
// to the separate ast-grep pass.
func textual(path string, data []byte, out io.Writer) int {
	var code strings.Builder
	for line := range strings.SplitSeq(string(data), "\n") {
		if !regexp.MustCompile(`^[\t\r\v\f ]*#`).MatchString(line) {
			code.WriteString(line)
			code.WriteByte('\n')
		}
	}
	git := regexp.MustCompile(`(^|[^-a-zA-Z0-9_])git `).MatchString(code.String())
	gh := regexp.MustCompile(`(^|[^-a-zA-Z0-9_])gh `).MatchString(code.String())
	count := 0
	if (git || gh) && !regexp.MustCompile(`(?m)^export LC_ALL=`).Match(data) {
		report(
			out,
			path,
			"1",
			"missing-locale-pin",
			"invokes git or gh without 'export LC_ALL=C'; sort order and [a-z] ranges shift with the locale",
		)
		count++
	}
	if gh && !regexp.MustCompile(`(?m)^unset .*GH_REPO`).Match(data) {
		report(
			out,
			path,
			"1",
			"missing-gh-unset",
			"invokes gh without unsetting GH_REPO and GH_HOST; an exported one retargets the repository",
		)
		count++
	}
	return count
}

func structural(ctx context.Context, root string, paths []string, run runner, out, stderr io.Writer) (int, int) {
	if len(paths) == 0 {
		return 0, 0
	}
	data, diagnostic, status := run(ctx, root, "ast-grep", append([]string{"scan", "--json=stream"}, paths...)...)
	if status > 1 || status < 0 {
		fmt.Fprintf(stderr, "check-script-hygiene: ast-grep failed with status %d\n%s", status, diagnostic)
		return 0, 1
	}
	lines, err := diagnostics(data)
	if err != nil {
		fmt.Fprintf(stderr, "check-script-hygiene: invalid ast-grep diagnostics: %s\n", err)
		return 0, 1
	}
	for _, line := range lines {
		const columns = 4
		fields := strings.SplitN(line, "\t", columns)
		report(out, fields[0], fields[1], fields[2], fields[3])
	}
	return len(lines), 0
}

func report(out io.Writer, path, line, rule, message string) {
	fmt.Fprintf(out, "%s:%s [error] %s  %s\n", path, line, rule, message)
}
