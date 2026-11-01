// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package retro preserves session evidence and profiles transcripts through session.py.
package retro

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	claudeHarness = "claude"
	codexHarness  = "codex"
)

var (
	errNoTranscript = errors.New("new-retro: no such transcript")
	errNoTemplate   = errors.New("new-retro: missing template")
	errNoRepository = errors.New("new-retro: not inside a git repository")
	errNoMetadata   = errors.New("new-retro: could not read session metadata")
	errBadFormat    = errors.New("new-retro: unsupported ANALYZE_SESSION_FORMAT")
)

// New runs the new-retro contract. Skill names the deployed analyze-session directory.
func New(ctx context.Context, skill string, args []string, out, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(
			stderr,
			`usage: agentrepo retro new --skill-dir <analyze-session> <transcript.jsonl> [extra-artifact ...]

Creates <main worktree>/.claude/retros/YYYYMMDD_<session id>/ (or .codex/retros
under Codex) holding RETRO.md plus artifacts/ (the transcript, the canned
session.py reports, and any extra files named on the command line).

The transcript may come from either harness. ANALYZE_SESSION_FORMAT=claude
or =codex overrides the format session.py would detect.
`,
		)
		const usageStatus = 2
		return usageStatus
	}
	summary, err := create(ctx, skill, args, stderr)
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			return exit.ExitCode()
		}
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprint(out, summary)
	return 0
}

func create(ctx context.Context, skill string, args []string, stderr io.Writer) (string, error) {
	skill, err := filepath.Abs(skill)
	if err != nil {
		return "", fmt.Errorf("new-retro: resolve skill directory: %w", err)
	}
	transcript := args[0]
	info, err := os.Stat(transcript)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s", errNoTranscript, transcript)
	}
	template := filepath.Join(skill, "templates/RETRO.md")
	body, err := os.ReadFile(template)
	if err != nil {
		return "", fmt.Errorf("%w: %s", errNoTemplate, template)
	}
	git := exec.CommandContext(ctx, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	git.Env = environment()
	common, err := git.Output()
	if err != nil {
		return "", errNoRepository
	}
	commonDir := strings.TrimSpace(string(common))
	metadata, err := report(ctx, skill, "meta", transcript, stderr)
	if err != nil {
		return "", err
	}
	values := make(map[string]string)
	for line := range strings.SplitSeq(string(metadata), "\n") {
		key, value, _ := strings.Cut(line, "=")
		values[key] = value
	}
	if values["SESSION_ID"] == "" || values["SESSION_DATE"] == "" {
		return "", errNoMetadata
	}
	base := ".claude/retros"
	if harness(skill) == codexHarness {
		base = ".codex/retros"
	}
	name := values["SESSION_DATE"] + "_" + values["SESSION_ID"]
	dir := filepath.ToSlash(filepath.Join(filepath.Dir(commonDir), base, name))
	artifacts := filepath.Join(dir, "artifacts")
	copied, err := preserve(ctx, skill, transcript, artifacts, args[1:], stderr)
	if err != nil {
		return "", err
	}
	state, err := scaffold(filepath.Join(dir, "RETRO.md"), string(body), values)
	if err != nil {
		return "", err
	}
	ignored, err := exclude(commonDir, base+"/")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"retro dir:  %s\nRETRO.md:   %s\nartifacts:  transcript.jsonl, 4 canned reports, %d extra\ngitignore:  %s\n",
		dir,
		state,
		copied,
		ignored,
	), nil
}

func report(ctx context.Context, skill, name, transcript string, stderr io.Writer) ([]byte, error) {
	format := "auto"
	switch value := os.Getenv("ANALYZE_SESSION_FORMAT"); value {
	case "", "auto":
	case claudeHarness:
		format = claudeHarness
	case codexHarness:
		format = codexHarness
	default:
		return nil, fmt.Errorf("%w: %q", errBadFormat, value)
	}
	script := filepath.Join(skill, "scripts/session.py")
	cmd := exec.CommandContext(ctx, "uv", "run", "--script", script, name, "--format", format, transcript)
	cmd.Env, cmd.Stderr = environment(), stderr
	data, err := cmd.Output()
	if err != nil {
		return data, fmt.Errorf("new-retro: run %s report: %w", name, err)
	}
	return data, nil
}

func environment() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "CDPATH", "GREP_OPTIONS", "LC_ALL", "PYTHONUTF8":
			continue
		}
		env = append(env, item)
	}
	return append(env, "LC_ALL=C", "PYTHONUTF8=1")
}

func harness(skill string) string {
	if value := os.Getenv("DEVTOOLS_HARNESS"); value != "" {
		return value
	}
	if os.Getenv("CLAUDE_CODE_SESSION_ID") != "" || os.Getenv("CLAUDECODE") != "" {
		return "claude"
	}
	if os.Getenv("CODEX_THREAD_ID") != "" {
		return codexHarness
	}
	path := "/" + filepath.ToSlash(skill) + "/"
	if strings.Contains(path, "/.agents/skills/") || strings.Contains(path, "/packages/agents/codex/") {
		return codexHarness
	}
	return "unknown"
}

func copyArtifact(src, dst string, follow bool) error {
	info, err := os.Lstat(src)
	if follow {
		info, err = os.Stat(src)
	}
	if err != nil {
		return fmt.Errorf("new-retro: read artifact: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return copySymlink(src, dst)
	}
	if info.IsDir() {
		return copyDirectory(src, dst)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("new-retro: read artifact: %w", err)
	}
	if err = writeFile(dst, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("new-retro: write artifact: %w", err)
	}
	return nil
}

const artifactPerm = 0o600

func writeFile(path string, data []byte, perm os.FileMode) error {
	dir, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	defer dir.Close()
	if err = dir.WriteFile(filepath.Base(path), data, perm); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func copyDirectory(src, dst string) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return fmt.Errorf("new-retro: copy directory: %w", err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("new-retro: read directory: %w", err)
	}
	for _, entry := range entries {
		if err = copyArtifact(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name()), false); err != nil {
			return err
		}
	}
	return nil
}

func copySymlink(src, dst string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return fmt.Errorf("new-retro: read artifact link: %w", err)
	}
	if err = os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("new-retro: remove artifact link: %w", err)
	}
	if err = os.Symlink(target, dst); err != nil {
		return fmt.Errorf("new-retro: copy artifact link: %w", err)
	}
	return nil
}

func scaffold(path, body string, values map[string]string) (string, error) {
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		return "left alone (already exists)", nil
	}
	body = strings.NewReplacer("{{SESSION_ID}}", values["SESSION_ID"], "{{RECORDS}}", values["RECORDS"], "{{SIZE_MB}}", values["SIZE_MB"], "{{ANALYZED}}", time.Now().Format("2006-01-02")).
		Replace(strings.TrimRight(body, "\n")) +
		"\n"
	if err := writeFile(path, []byte(body), artifactPerm); err != nil {
		return "", fmt.Errorf("new-retro: write RETRO.md: %w", err)
	}
	return "scaffolded from template", nil
}

func exclude(commonDir, entry string) (string, error) {
	path := filepath.Join(commonDir, "info/exclude")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("new-retro: create exclude directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return "", fmt.Errorf("new-retro: open exclude: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("new-retro: read exclude: %w", err)
	}
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		if string(line) == entry || string(line) == strings.TrimSuffix(entry, "/") {
			return "already ignored", nil
		}
	}
	if _, err = fmt.Fprintf(
		file,
		"\n# Session retrospectives. Local working notes, never committed.\n%s\n",
		entry,
	); err != nil {
		return "", fmt.Errorf("new-retro: write exclude: %w", err)
	}
	return "added to .git/info/exclude", nil
}

func preserve(
	ctx context.Context,
	skill, transcript, artifacts string,
	extras []string,
	stderr io.Writer,
) (int, error) {
	if err := os.MkdirAll(artifacts, 0o700); err != nil {
		return 0, fmt.Errorf("new-retro: create artifacts: %w", err)
	}
	if err := copyArtifact(transcript, filepath.Join(artifacts, "transcript.jsonl"), true); err != nil {
		return 0, err
	}
	for _, name := range []string{"index", "errors", "loops", "lint"} {
		data, reportErr := report(ctx, skill, name, transcript, stderr)
		if reportErr != nil {
			return 0, reportErr
		}
		if err := os.WriteFile(filepath.Join(artifacts, name+".txt"), data, 0o600); err != nil {
			return 0, fmt.Errorf("new-retro: write report: %w", err)
		}
	}
	copied := 0
	for _, extra := range extras {
		if _, statErr := os.Lstat(extra); statErr != nil {
			_, _ = fmt.Fprintf(stderr, "new-retro: skipping missing artifact: %s\n", extra)
			continue
		}
		if err := copyArtifact(
			extra,
			filepath.Join(artifacts, filepath.Base(filepath.Clean(extra))),
			false,
		); err != nil {
			return 0, err
		}
		copied++
	}

	return copied, nil
}
