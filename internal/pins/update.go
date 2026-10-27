// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package pins updates consumer release pins and refreshes the delivered payload.
package pins

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

type runner func(context.Context, string, []string, bool, io.Writer, io.Writer) int

type updater struct {
	dir         string
	out, stderr io.Writer
	run         runner
}

// Run updates pins in dir (the current directory when empty). Later failures
// keep the completed rewrites, matching the consumer shell task.
func Run(ctx context.Context, dir, version string, out, stderr io.Writer) int {
	u := updater{dir: dir, out: out, stderr: stderr}
	u.run = u.command
	return u.update(ctx, version)
}

func (u *updater) command(ctx context.Context, name string, args []string, scrub bool, out, stderr io.Writer) int {
	var cmd *exec.Cmd
	switch name {
	case "git":
		cmd = exec.CommandContext(ctx, "git")
	case "mise":
		cmd = exec.CommandContext(ctx, "mise")
	case "vendir":
		cmd = exec.CommandContext(ctx, "vendir")
	}

	// Keep executable selection fixed and pass consumer values as separate arguments.
	cmd.Args = append(cmd.Args, args...)
	cmd.Dir = u.dir
	cmd.Stdout, cmd.Stderr = out, stderr
	cmd.Stdin = os.Stdin
	if scrub {
		for _, v := range os.Environ() {
			key, _, _ := strings.Cut(v, "=")
			switch key {
			case "GIT_DIR", "GIT_COMMON_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY":
				continue
			}
			cmd.Env = append(cmd.Env, v)
		}
	}
	if err := cmd.Run(); err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			return exit.ExitCode()
		}
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return commandUnavailable
	}
	return 0
}

const (
	commandUnavailable = 127
	apmReadFailure     = 2
	guardTool          = "guard-markdown"
)

const remote = "https://github.com/tbhb-dev/devtools"

func (u *updater) update(ctx context.Context, version string) int {
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	if status := u.validate(); status != 0 {
		return status
	}
	var sha string
	for _, ref := range []string{"refs/tags/" + version + "^{}", "refs/tags/" + version} {
		var out bytes.Buffer
		if status := u.run(ctx, "git", []string{"ls-remote", remote, ref}, false, &out, u.stderr); status != 0 {
			return status
		}
		// cut -f1 preserves lines without tabs. Command substitution strips newlines.
		var fields []string
		for line := range strings.SplitSeq(strings.TrimRight(out.String(), "\n"), "\n") {
			first, _, _ := strings.Cut(line, "\t")
			fields = append(fields, first)
		}
		sha = strings.Join(fields, "\n")
		if sha != "" {
			break
		}
	}
	if sha == "" {
		_, _ = fmt.Fprintf(u.stderr, "no tag %s at %s\n", version, remote)
		return 1
	}
	_, _ = fmt.Fprintf(u.out, "%s resolves to %s\n", version, sha)
	if err := u.rewrite(version, sha); err != nil {
		_, _ = fmt.Fprintln(u.stderr, err)
		return 1
	}
	return u.refresh(ctx, version)
}

func (u *updater) validateAPM() int {
	if u.exists("apm.yml") {
		body, err := os.ReadFile(filepath.Join(u.dir, "apm.yml"))
		if err != nil {
			_, _ = fmt.Fprintln(u.stderr, err)
			return apmReadFailure
		}
		site := regexp.MustCompile(`^[[:space:]]*-[[:space:]]*["']?tbhb-dev/devtools`)
		supported := regexp.MustCompile(
			`tbhb-dev/devtools(/packages/agents/(common|claude|codex|agy))?#v[0-9]+\.[0-9]+\.[0-9]+`,
		)
		bad := false
		for line := range strings.SplitSeq(string(body), "\n") {
			if site.MatchString(line) && !supported.MatchString(line) {
				_, _ = fmt.Fprintln(u.stderr, "unsupported devtools APM pin: "+line)
				bad = true
			}
		}
		if bad {
			return 1
		}
	}
	return 0
}

func (u *updater) validate() int {
	if status := u.validateAPM(); status != 0 {
		return status
	}
	body, err := os.ReadFile(filepath.Join(u.dir, "mise.toml"))
	if err != nil {
		_, _ = fmt.Fprintln(u.stderr, err)
	}
	guard := ""
	for _, tool := range []string{guardTool, "commit-lint", "gh-post"} {
		key := `"go:github.com/tbhb-dev/devtools/cmd/` + tool + `"`
		decl := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `[[:blank:]]*=`)
		if !decl.Match(body) {
			if tool == guardTool {
				_, _ = fmt.Fprintln(u.stderr, `declare `+key+` in mise.toml [tools]`)
			} else {
				_, _ = fmt.Fprintf(u.stderr, "%s = %q\n", key, guard)
			}
			return 1
		}
		if tool == guardTool {
			match := regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(key)+`[[:blank:]]*=[[:blank:]]*"([^"]*)"`).
				FindAllSubmatch(body, -1)
			var versions []string
			for _, m := range match {
				versions = append(versions, string(m[1]))
			}
			guard = strings.Join(versions, "\n")
		}
	}
	return 0
}

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

func (u *updater) refresh(ctx context.Context, version string) int {
	if status := u.run(ctx, "vendir", []string{"sync"}, true, u.out, u.stderr); status != 0 {
		return status
	}
	if platforms := os.Getenv("DEVTOOLS_LOCK_PLATFORMS"); platforms != "" {
		if status := u.run(
			ctx,
			"mise",
			[]string{"lock", "--platform", platforms},
			false,
			u.out,
			u.stderr,
		); status != 0 {
			return status
		}
	} else {
		_, _ = fmt.Fprintln(u.stderr, "DEVTOOLS_LOCK_PLATFORMS is unset, so the lockfiles were left alone")
		_, _ = fmt.Fprintln(
			u.stderr,
			"         set it in the root mise.toml [env], then run 'mise lock --platform <list>'",
		)
	}
	for _, args := range [][]string{{"install"}, {"run", "devtools:apm-sync"}} {
		if status := u.run(ctx, "mise", args, false, u.out, u.stderr); status != 0 {
			return status
		}
	}
	var tasks bytes.Buffer
	status := u.run(ctx, "mise", []string{"tasks", "ls", "--all"}, false, &tasks, io.Discard)
	if status == 0 && regexp.MustCompile(`(?m)^devtools:check-pins[[:blank:]]`).Match(tasks.Bytes()) {
		return u.run(ctx, "mise", []string{"run", "devtools:check-pins"}, false, u.out, u.stderr)
	}
	_, _ = fmt.Fprintf(u.stderr, "%s does not ship devtools:check-pins, so the rewrite went unverified\n", version)
	return 0
}
