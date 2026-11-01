// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package testaffected selects repository test suites conservatively.
package testaffected

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	full        = "test-full"
	goSuite     = "test-go"
	pythonSuite = "test-py"
)

var errContext = errors.New("cannot determine affected paths")

// Run executes the selection through the existing mise tasks.
func Run(ctx context.Context, out, diagnostic io.Writer) int {
	root, err := gitOutput(ctx, ".", "rev-parse", "--show-toplevel")
	if err != nil {
		root = "."
	}
	err = execute(ctx, root, out, func(task string) error {
		cmd := exec.CommandContext(ctx, "mise", "run", task)
		cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = root, os.Stdin, out, diagnostic
		if runErr := cmd.Run(); runErr != nil {
			return fmt.Errorf("%s: %w", task, runErr)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(diagnostic, err)
		return 1
	}
	return 0
}

func execute(ctx context.Context, root string, out io.Writer, run func(string) error) error {
	paths, err := changedPaths(ctx, root)
	tasks := []string{full}
	if err == nil {
		tasks = selectPaths(paths)
	}
	if len(tasks) == 0 {
		fmt.Fprintln(out, "No affected tests selected.")
		return nil
	}
	fmt.Fprintln(out, "Selected suites:", strings.Join(tasks, ", "))
	for _, task := range tasks {
		if runErr := run(task); runErr != nil {
			return runErr
		}
	}
	return nil
}

func selectPaths(paths []string) []string {
	selected := make(map[string]bool)
	for _, path := range paths {
		tasks := pathTasks(path)
		if len(tasks) == 1 && tasks[0] == full {
			return tasks
		}
		for _, task := range tasks {
			selected[task] = true
		}
	}
	var tasks []string
	for _, task := range []string{goSuite, pythonSuite} {
		if selected[task] {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func pathTasks(path string) []string {
	// Infrastructure and policy are intentionally absent from the narrow map.
	// Changes there, including this selector and its tests, always run everything.
	switch {
	case path == "README.md", strings.HasPrefix(path, "docs/") && strings.HasSuffix(path, ".md"):
		return nil
	case strings.HasPrefix(path, "packages/devtools/") && strings.HasSuffix(path, ".py") && filepath.Base(path) != "conftest.py":
		return []string{pythonSuite}
	case (strings.HasPrefix(path, "internal/buildmeta/") || strings.HasPrefix(path, "cmd/agy-otel/")) && strings.HasSuffix(path, ".go"):
		return []string{goSuite}
	default:
		return []string{full}
	}
}

func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	data, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git: %w", err)
	}
	return strings.TrimSuffix(string(data), "\n"), nil
}

func changedPaths(ctx context.Context, root string) ([]string, error) {
	// A failed tip lookup (including an unborn branch) can't narrow coverage.
	if _, err := gitOutput(ctx, root, "rev-parse", "--verify", "HEAD"); err != nil {
		return nil, err
	}
	unmerged, err := gitOutput(ctx, root, "ls-files", "--unmerged", "-z")
	if err != nil {
		return nil, err
	}
	if unmerged != "" {
		return nil, errContext
	}
	staged, err := gitOutput(ctx, root, "diff", "--cached", "--name-only", "-z", "--no-renames", "HEAD", "--")
	if err != nil {
		return nil, err
	}
	if staged != "" {
		return splitPaths(staged), nil
	}
	// Publication requires a clean tree. Unexpected local work is uncertain.
	status, err := gitOutput(ctx, root, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return nil, err
	}
	if status != "" {
		return nil, errContext
	}
	base, err := baseRef(ctx, root)
	if err != nil {
		return nil, err
	}
	ancestor, err := gitOutput(ctx, root, "merge-base", "HEAD", base)
	if err != nil {
		return nil, err
	}
	changed, err := gitOutput(ctx, root, "diff", "--name-only", "-z", "--no-renames", ancestor, "HEAD", "--")
	if err != nil {
		return nil, err
	}
	return splitPaths(changed), nil
}

func splitPaths(data string) []string {
	if data == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(data, "\x00"), "\x00")
}

func baseRef(ctx context.Context, root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "PR_AGENTDESC.md"))
	var base string
	switch {
	case err == nil:
		base, err = draftBase(data)
		if err != nil {
			return "", err
		}
	case errors.Is(err, os.ErrNotExist):
		ref, refErr := gitOutput(ctx, root, "symbolic-ref", "refs/remotes/origin/HEAD")
		if refErr != nil {
			return "", refErr
		}
		base = strings.TrimPrefix(ref, "refs/remotes/origin/")
	default:
		return "", fmt.Errorf("read draft: %w", err)
	}
	if base == "" {
		return "", errContext
	}
	if _, formatErr := gitOutput(ctx, root, "check-ref-format", "refs/heads/"+base); formatErr != nil {
		return "", formatErr
	}
	for _, ref := range []string{"refs/remotes/origin/" + base, "refs/heads/" + base} {
		if oid, refErr := gitOutput(ctx, root, "rev-parse", "--verify", ref+"^{commit}"); refErr == nil {
			return oid, nil
		}
	}
	return "", errContext
}

func draftBase(data []byte) (string, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) < 2 || lines[0] != "---" {
		return "", errContext
	}
	for i := 1; i < len(lines); i++ {
		if lines[i] != "---" {
			continue
		}
		var draft struct {
			Base string `yaml:"base"`
		}
		if parseErr := yaml.Unmarshal([]byte(strings.Join(lines[1:i], "\n")), &draft); parseErr != nil {
			return "", errContext
		}
		if draft.Base == "" {
			return "", errContext
		}
		return draft.Base, nil
	}
	return "", errContext
}
