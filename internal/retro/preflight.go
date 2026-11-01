// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package retro

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Preflight resolves a transcript and prints the seven-section session profile.
func Preflight(ctx context.Context, skill string, args []string, out, stderr io.Writer) int {
	if _, err := exec.LookPath("uv"); err != nil {
		_, _ = fmt.Fprint(
			out,
			"\n== uv ==\nMISSING — session.py selects its interpreter through a uv script\nshebang, so every report here needs uv on PATH.\n\nInstall it, then run this preflight again:\n\n  brew install uv\n",
		)
		return 0
	}
	arg := ""
	if len(args) > 0 {
		arg = args[0]
	}
	transcript, reason := resolve(arg)
	if reason != "" {
		unresolved(out, skill, reason)
		return 0
	}
	transcript, err := filepath.Abs(transcript)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	file, err := os.Open(transcript)
	if err != nil {
		unresolved(out, skill, "cannot read: "+transcript)
		return 0
	}
	first := make([]byte, 1)
	_, err = file.Read(first)
	_ = file.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		unresolved(out, skill, "cannot read: "+transcript)
		return 0
	}
	if first[0] != '{' {
		unresolved(out, skill, "not JSON lines: "+transcript)
		return 0
	}
	_, _ = fmt.Fprintf(out, "\n== transcript ==\n%s\n\n== retro ==\n", filepath.ToSlash(transcript))
	var summary bytes.Buffer
	if New(ctx, skill, []string{transcript}, &summary, stderr) != 0 {
		_, _ = fmt.Fprintln(out, "scaffolding failed — tell the operator")
		return 0
	}
	_, _ = fmt.Fprint(out, summary.String())
	printProfile(out, stderr, summary.String())
	return 0
}

func printProfile(out, stderr io.Writer, summary string) {
	const indexRows, lintRows = 24, 14
	first, _, _ := strings.Cut(summary, "\n")
	dir := strings.TrimPrefix(first, "retro dir:  ")
	artifacts := filepath.Join(dir, "artifacts")
	rows := 12
	if value := os.Getenv("ANALYZE_PREFLIGHT_LOOPS"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			rows = parsed
		} else {
			rows = 0
		}
	}
	profile(out, artifacts, "index", indexRows, "")
	profile(out, artifacts, "loops", rows, "Ranked by rounds. Each round beyond the first is a wasted trip.\n\n")
	profile(out, artifacts, "lint", lintRows, "")
	_, _ = fmt.Fprintf(out, "\n== artifacts ==\n%s\n", filepath.ToSlash(artifacts))
	entries, err := os.ReadDir(artifacts) // ReadDir sorts names. List any readable entries as find does.
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "preflight: list artifacts: %v\n", err)
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			_, _ = fmt.Fprintf(out, "  %s\n", entry.Name())
		}
	}
	_, _ = fmt.Fprint(
		out,
		"\n== next ==\nThe transcript is resolved, the retro is open, and the reports are on\ndisk. Start at step 1 of the skill body, reading only what these line\nnumbers point at.\n",
	)
}

func resolve(arg string) (string, string) {
	if arg == "" {
		return "", "no path given"
	}
	if info, err := os.Stat(arg); err == nil && info.Mode().IsRegular() {
		return arg, ""
	}
	id := strings.TrimSuffix(arg, ".jsonl")
	var matches []string
	for _, store := range []string{".claude/projects", ".codex/sessions"} {
		matches = append(
			matches,
			findTranscripts(filepath.Join(os.Getenv("HOME"), store), id, store == ".claude/projects")...,
		)
	}
	if len(matches) == 1 {
		return matches[0], ""
	}
	if len(matches) > 1 {
		return "", "the id matches more than one transcript; pass a full path"
	}
	return "", "not a readable file: " + arg
}

func profile(out io.Writer, artifacts, name string, limit int, intro string) {
	_, _ = fmt.Fprintf(out, "\n== %s ==\n%s", name, intro)
	data, err := os.ReadFile(filepath.Join(artifacts, name+".txt"))
	if err != nil {
		_, _ = fmt.Fprintln(out, "(missing)")
		return
	}
	for line := range strings.SplitAfterSeq(string(data), "\n") {
		if limit <= 0 {
			break
		}
		_, _ = fmt.Fprint(out, line)
		limit--
	}
}

func unresolved(out io.Writer, skill, reason string) {
	_, _ = fmt.Fprintf(out, `
== transcript ==
UNRESOLVED — %s

== what to do ==
Do not open transcripts to search them. Reading transcripts to find
one costs more context than the analysis that follows.

Dispatch a cheap delegated agent to locate it, following the template in
%s/references/LOCATE_SESSION.md. Give it the fingerprint the operator
described: a filename the session created, an identifier, or a rule name
out of its output.

Then run this preflight again with the path it returns:

%s%s/scripts/preflight.sh <path>

Claude Code keeps transcripts under ~/.claude/projects/<project>/. A session
running inside a git worktree writes under the MAIN repository's project
directory, and the worktree's own directory may exist while holding no
.jsonl at all. Codex keeps rollouts under date directories in
~/.codex/sessions. Do not infer either path from the repository or
worktree path.
`, reason, skill, "  bash ", skill)
}

func findTranscripts(root, id string, claude bool) []string {
	var matches []string
	store, err := os.OpenRoot(root)
	if err != nil {
		// An absent or unreadable store contributes no hits.
		return matches
	}
	defer store.Close()
	err = fs.WalkDir(store.FS(), ".", func(rel string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fs.SkipDir
		}
		if claude && strings.Count(rel, "/") > 1 {
			return fs.SkipDir
		}
		match := strings.HasSuffix(entry.Name(), id+".jsonl")
		if claude {
			match = entry.Name() == id+".jsonl"
		}
		if match && entry.Type().IsRegular() {
			matches = append(matches, filepath.Join(root, filepath.FromSlash(rel)))
		}
		return nil
	})
	if err != nil {
		// The callback only skips entries. After a walk error, the hits are unreliable.
		return nil
	}
	return matches
}
