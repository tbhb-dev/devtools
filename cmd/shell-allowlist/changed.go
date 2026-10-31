// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// changedFiles selects whole scripts relative to the merge base, including local edits.
func changedFiles(base string) ([]string, error) {
	mergeBase, err := gitOutput("merge-base", base, "HEAD")
	if err != nil {
		return nil, err
	}
	changed, err := gitOutput("diff", "--name-only", "-z", "--diff-filter=d", strings.TrimSpace(mergeBase), "--")
	if err != nil {
		return nil, err
	}
	untracked, err := gitOutput("ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, path := range scriptPaths(changed + untracked) {
		if strings.HasSuffix(path, ".bats") {
			continue
		}
		if strings.HasPrefix(path, "vendor/") || strings.HasPrefix(path, "tests/fixtures/") ||
			generatedSkillCopy(path) {
			continue
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func scriptPaths(files string) []string {
	var paths []string
	for path := range strings.SplitSeq(files, "\x00") {
		if strings.HasSuffix(path, ".sh") || strings.HasSuffix(path, ".bash") || strings.HasSuffix(path, ".bats") {
			paths = append(paths, path)
		}
	}
	return paths
}

func generatedSkillCopy(path string) bool {
	for prefix, harness := range map[string]string{
		"packages/agents/claude/.apm/skills/": "claude",
		"packages/agents/codex/.apm/skills/":  "codex",
		".claude/skills/":                     "claude",
		".agents/skills/":                     "codex",
	} {
		if rest, ok := strings.CutPrefix(path, prefix); ok {
			name, _, _ := strings.Cut(rest, "/")
			_, err := os.Stat("packages/agents/source/skills/" + name + "/SKILL." + harness + ".md")
			return err == nil
		}
	}
	return false
}

func gitOutput(args ...string) (string, error) {
	//nolint:gosec // G702: Git receives separate arguments; the executable is fixed and no shell runs.
	out, err := exec.CommandContext(context.Background(), "git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return string(out), nil
}
