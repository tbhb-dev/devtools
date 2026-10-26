// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package pins checks consumer devtools pins without resolving or rewriting them.
package pins

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type site struct{ version, sha, location string }

var (
	apm        = regexp.MustCompile(`^[[:space:]]*-[[:space:]]*["']?tbhb-dev/devtools`)
	apmVersion = regexp.MustCompile(
		`.*tbhb-dev/devtools(/packages/agents/(common|claude|codex|agy))?#(v[^[:space:]"']*)`,
	)
	tool = regexp.MustCompile(
		`^"go:github.com/tbhb-dev/devtools/cmd/(guard-markdown|commit-lint|agentrepo|gh-post)"[[:space:]]*=`,
	)
	workflow        = regexp.MustCompile(`uses:[[:space:]]*tbhb-dev/devtools/`)
	workflowVersion = regexp.MustCompile(`#[[:space:]]*v`)
	preset          = regexp.MustCompile(`github>tbhb-dev/devtools//\.github/[^/"]+\.json5`)
	presetRef       = regexp.MustCompile(`.*[^/"]+\.json5#([^"]*)`)
	presetVersion   = regexp.MustCompile(`//[[:space:]]*v`)
	block           = regexp.MustCompile(`(url|repo):[[:space:]]*https://github\.com/tbhb-dev/devtools([[:space:]]|$)`)
	ref             = regexp.MustCompile(`^[[:space:]]*ref:[[:space:]]*`)
	rev             = regexp.MustCompile(`^[[:space:]]*rev:[[:space:]]*`)
	frozen          = regexp.MustCompile(`frozen:[[:space:]]*v`)
	proseVersion    = regexp.MustCompile(`tbhb-dev/devtools[[:space:]]+(v[0-9]+\.[0-9]+\.[0-9]+)$`)
	space           = regexp.MustCompile(`[[:space:]]`)
	quoteSpace      = regexp.MustCompile(`[[:space:]"']`)
	nonHex          = regexp.MustCompile(`[^0-9a-f]`)
)

func before(re *regexp.Regexp, value string) string {
	if at := re.FindStringIndex(value); at != nil {
		return value[:at[0]]
	}
	return value
}

func comment(line, marker string, end *regexp.Regexp) string {
	// The frozen checker's greedy substitution selects the last marker.
	re := regexp.MustCompile(`.*` + marker + `[[:space:]]*`)
	return before(end, re.ReplaceAllString(line, ""))
}

func discover(file, text string) []site {
	var sites []site
	inBlock := false
	for i, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		emit := func(version, sha string) { sites = append(sites, site{version, sha, fmt.Sprintf("%s:%d", file, i+1)}) }
		discoverLine(file, line, emit)
		if block.MatchString(line) {
			inBlock = true
			continue
		}
		if inBlock && ref.MatchString(line) {
			emit(comment(line, "ref:", space), "-")
			inBlock = false
		}
		if inBlock && rev.MatchString(line) {
			version := versionComment(line, frozen, "frozen:", space)
			emit(version, comment(line, "rev:", nonHex))
			inBlock = false
		}
	}
	return sites
}

func versionComment(line string, pattern *regexp.Regexp, marker string, end *regexp.Regexp) string {
	if pattern.MatchString(line) {
		return comment(line, marker, end)
	}
	return "?"
}

func discoverLine(file, line string, emit func(string, string)) {
	if file == "apm.yml" && apm.MatchString(line) {
		version := "?"
		if match := apmVersion.FindStringSubmatch(line); match != nil {
			version = before(quoteSpace, match[3])
		}
		emit(version, "-")
	}
	if tool.MatchString(line) {
		value := regexp.MustCompile(`^[^=]*=[[:space:]]*"`).ReplaceAllString(line, "")
		value, _, _ = strings.Cut(value, `"`)
		emit(value, "-")
	}
	if workflow.MatchString(line) {
		sha := before(nonHex, line[strings.LastIndex(line, "@")+1:])
		emit(versionComment(line, workflowVersion, "#", space), sha)
	}
	if preset.MatchString(line) {
		discoverPreset(line, emit)
	}
}

func discoverPreset(line string, emit func(string, string)) {
	value := ""
	if match := presetRef.FindStringSubmatch(line); match != nil {
		value = match[1]
	}
	switch {
	case value == "":
		emit("unpinned", "-")
	case strings.HasPrefix(value, "v"):
		emit(value, "-")
	default:
		emit(versionComment(line, presetVersion, "//", regexp.MustCompile(`[[:space:],]`)), value)
	}
}

// Run checks pins relative to dir, preserving the consumer task's output streams.
// An empty dir uses the current working directory.
func Run(ctx context.Context, dir string, out, stderr io.Writer) int {
	if dir == "" {
		dir = "."
	}
	sites, mise, err := readSites(dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	guard, status := required(mise, stderr)
	if status != 0 {
		return status
	}
	versions, status := compare(sites, out, stderr)
	if status != 0 {
		return status
	}
	if status = checkProse(ctx, dir, guard, stderr); status != 0 {
		return status
	}
	_, _ = fmt.Fprintf(out, "every devtools pin is on %s\n", versions)
	return 0
}

func readSites(dir string) ([]site, string, error) {
	files := []string{"mise.toml", "apm.yml", "vendir.yml", ".pre-commit-config.yaml", ".github/renovate.json5"}
	entries, err := os.ReadDir(filepath.Join(dir, ".github", "workflows"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, "", fmt.Errorf("discover workflows: %w", err)
	}
	for _, suffix := range []string{".yml", ".yaml"} {
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".") && strings.HasSuffix(entry.Name(), suffix) {
				files = append(files, ".github/workflows/"+entry.Name())
			}
		}
	}
	var sites []site
	var mise string
	for _, file := range files {
		data, readErr := readRegular(filepath.Join(dir, filepath.FromSlash(file)))
		if readErr != nil {
			return nil, "", readErr
		}
		sites = append(sites, discover(file, string(data))...)
		if file == "mise.toml" {
			mise = string(data)
		}
	}
	return sites, mise, nil
}

func readRegular(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("discover pins: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pins: %w", err)
	}
	return data, nil
}

func checkProse(ctx context.Context, dir, guard string, stderr io.Writer) int {
	// git grep searches tracked working-tree text, including modifications. A
	// producer failure must discard its partial stdout before interpreting it.
	cmd := exec.CommandContext(
		ctx,
		"git",
		"grep",
		"-nI",
		"-o",
		"-E",
		`tbhb-dev/devtools[[:space:]]+v[0-9]+\.[0-9]+\.[0-9]+`,
		"--",
	)
	cmd.Dir, cmd.Stderr = dir, stderr
	data, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		if exit.ExitCode() != 1 {
			return exit.ExitCode()
		}
		// Exit 1 is the documented no-match result.
	}
	bad := false
	for mention := range strings.SplitSeq(strings.TrimSuffix(string(data), "\n"), "\n") {
		match := proseVersion.FindStringSubmatchIndex(mention)
		if match == nil {
			continue
		}
		version := mention[match[2]:match[3]]
		if version != guard {
			_, _ = fmt.Fprintf(
				stderr,
				"%s: prose mentions %s; pinned %s (edit manually)\n",
				strings.TrimSuffix(mention[:match[0]], ":"),
				version,
				guard,
			)
			bad = true
		}
	}
	if bad {
		return 1
	}
	return 0
}

func required(mise string, stderr io.Writer) (string, int) {
	has := func(name string) bool {
		pattern := regexp.MustCompile(`^"go:github.com/tbhb-dev/devtools/cmd/` + name + `"[[:space:]]*=`)
		for line := range strings.SplitSeq(mise, "\n") {
			if pattern.MatchString(line) {
				return true
			}
		}
		return false
	}
	if !has("agentrepo") {
		_, _ = fmt.Fprintln(
			stderr,
			`recommend declaring "go:github.com/tbhb-dev/devtools/cmd/agentrepo" in mise.toml [tools] for bounded reviews`,
		)
	}
	if !has("guard-markdown") {
		_, _ = fmt.Fprintln(stderr, "missing guard-markdown tool pin in mise.toml [tools]")
		return "", 1
	}
	var guards []string
	re := regexp.MustCompile(`^"go:github.com/tbhb-dev/devtools/cmd/guard-markdown"[[:space:]]*=[[:space:]]*"([^"]*)"`)
	for line := range strings.SplitSeq(mise, "\n") {
		if match := re.FindStringSubmatchIndex(line); match != nil {
			guards = append(guards, line[match[2]:match[3]]+line[match[1]:])
		}
	}
	guard := strings.Join(guards, "\n")
	for _, name := range []string{"commit-lint", "gh-post"} {
		if !has(name) {
			_, _ = fmt.Fprintf(
				stderr,
				"missing %s tool pin in mise.toml [tools]; add exactly:\n\"go:github.com/tbhb-dev/devtools/cmd/%s\" = \"%s\"\n",
				name,
				name,
				guard,
			)
			return "", 1
		}
	}
	return guard, 0
}

func compare(sites []site, out, stderr io.Writer) (string, int) {
	if len(sites) == 0 {
		_, _ = fmt.Fprintln(
			stderr,
			"no tbhb-dev/devtools pins found -- check-pins verified nothing\n         run it from a repository that vendors devtools",
		)
		return "", 1
	}
	versions, shas := map[string]bool{}, map[string]bool{}
	for _, s := range sites {
		_, _ = fmt.Fprintf(out, "  %-10s %-42s %s\n", s.version, s.sha, s.location)
		versions[s.version] = true
		if s.sha != "-" {
			shas[s.sha] = true
		}
	}
	bad := false
	if len(versions) != 1 || versions["?"] {
		report(stderr, sites, "version", versions)
		bad = true
	}
	if len(shas) > 1 {
		report(stderr, sites, "commit", shas)
		bad = true
	}
	if bad {
		_, _ = fmt.Fprintln(
			stderr,
			"         run 'mise run devtools:update-pins <version>' to move every site together",
		)
		return "", 1
	}
	return strings.Join(keys(versions), "\n"), 0
}

func keys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func report(stderr io.Writer, sites []site, label string, values map[string]bool) {
	_, _ = fmt.Fprintf(stderr, "devtools pins disagree on the %s:\n", label)
	for _, value := range keys(values) {
		_, _ = fmt.Fprintf(stderr, "  %s\n", value)
		for _, s := range sites {
			actual := s.version
			if label == "commit" {
				actual = s.sha
			}
			if actual == value {
				_, _ = fmt.Fprintf(stderr, "    %s\n", s.location)
			}
		}
	}
}
