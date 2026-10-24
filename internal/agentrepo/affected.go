// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

var errAffectedBase = errors.New("affected tests: --base is required")

func affectedGit(cmd *cobra.Command, args ...string) (string, error) {
	output, err := exec.CommandContext(cmd.Context(), "git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("affected tests: git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSuffix(string(output), "\n"), nil
}

func affectedProcess(cmd *cobra.Command, name string, args ...string) error {
	process := exec.CommandContext(cmd.Context(), name, args...)
	process.Stdin = cmd.InOrStdin()
	process.Stdout = cmd.OutOrStdout()
	process.Stderr = cmd.ErrOrStderr()
	if err := process.Run(); err != nil {
		return fmt.Errorf("affected tests: %s: %w", name, err)
	}
	return nil
}

func testAffectedCommand() *cobra.Command {
	var base string
	var staged bool
	cmd := &cobra.Command{
		Use:   "test-affected --base REV [--staged]",
		Short: "Run devtools suites covering the committed or indexed diff",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if base == "" {
				return errAffectedBase
			}
			args := []string{"diff", "--no-ext-diff", "--no-renames", "--name-only", "-z"}
			if staged {
				args = append(args, "--cached")
			}
			args = append(args, base)
			if !staged {
				args = append(args, "HEAD")
			}
			changed, err := affectedGit(cmd, append(args, "--")...)
			if err != nil {
				return err
			}
			tasks := affectedTasks(strings.Split(changed, "\x00"))
			if len(tasks) == 0 {
				cmd.Println("No affected tests were selected.")
			}
			for _, task := range tasks {
				if taskErr := runAffectedTask(cmd, task); taskErr != nil {
					return taskErr
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Revision to compare with HEAD or the index (required)")
	cmd.Flags().BoolVar(&staged, "staged", false, "Compare with the index instead of HEAD")
	return cmd
}

func runAffectedTask(cmd *cobra.Command, task string) error {
	args := append([]string{"run"}, strings.Fields(task)...)
	if strings.HasPrefix(task, "go test ") {
		args = append([]string{"exec", "--"}, strings.Fields(task)...)
	}
	return affectedProcess(cmd, "mise", args...)
}

func affectedTasks(paths []string) []string {
	tasks := make(map[string]bool)
	for _, file := range paths {
		if file != "" {
			selectAffected(tasks, file)
		}
	}
	if tasks["test"] {
		return []string{"test"}
	}
	if tasks["test-go"] {
		removeAffectedPrefix(tasks, "go test ")
	}
	if tasks["test-hooks tests"] {
		removeAffectedPrefix(tasks, "test-hooks tests/")
	}
	result := make([]string, 0, len(tasks))
	for task := range tasks {
		if dir, ok := strings.CutPrefix(task, "test-hooks "); ok {
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				return []string{"test"}
			}
		}
		result = append(result, task)
	}
	slices.Sort(result)
	return result
}

func removeAffectedPrefix(tasks map[string]bool, prefix string) {
	for task := range tasks {
		if strings.HasPrefix(task, prefix) {
			delete(tasks, task)
		}
	}
}

func selectAffected(tasks map[string]bool, file string) {
	if affectedFullSuiteInput(file) {
		tasks["test"] = true
		return
	}
	if strings.HasPrefix(file, "tests/") {
		selectTestInputs(tasks, file)
		return
	}
	if pkg := affectedGoPackage(file); pkg != "" {
		tasks["go test ./"+pkg] = true
		// Bats builds and exercises Go commands and their dependencies too.
		tasks["test-hooks tests"] = true
		return
	}
	selectHelperTests(tasks, file)
	switch {
	case file == "go.mod", file == "go.sum", strings.HasPrefix(file, "vendor/"):
		tasks["test-go"] = true
		// Bats builds and exercises Go commands and their dependencies too.
		tasks["test-hooks tests"] = true
	case strings.HasPrefix(file, "packages/agents/"),
		strings.HasPrefix(file, ".agents/skills/"), strings.HasPrefix(file, ".claude/skills/"):
		selectSkillTests(tasks, file)
	case file == "pyproject.toml", file == "uv.lock":
		// Shell suites also run Python with this environment.
		tasks["test-py"] = true
		tasks["test-hooks tests"] = true
	case strings.HasSuffix(file, ".py"):
		tasks["test-py"] = true
	case (strings.HasPrefix(file, "scripts/") || strings.HasPrefix(file, "tools/")) &&
		(strings.HasSuffix(file, ".sh") || file == "tools/mise-bootstrap"):
		dir, _, _ := strings.Cut(file, "/")
		tasks["test-hooks tests/"+dir] = true
	case strings.HasPrefix(file, "docs/") && strings.HasSuffix(file, ".md"):
		// Only documentation with no suite owner may skip tests.
	default:
		// An unowned input may affect any suite, including repository contracts.
		tasks["test"] = true
	}
}

func affectedFullSuiteInput(file string) bool {
	return file == "mise.toml" || strings.HasPrefix(file, ".config/mise/") ||
		strings.HasPrefix(file, ".devtools/")
}

func affectedGoPackage(file string) string {
	for dir := path.Dir(file); ; dir = path.Dir(dir) {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
					return dir
				}
			}
		}
		if dir == "." {
			return ""
		}
	}
}

func selectHelperTests(tasks map[string]bool, file string) {
	// These helpers are exercised outside their usual directory mirrors.
	switch file {
	case "tools/skill-tokens.sh":
		tasks["test-hooks tests/scripts"] = true
	case "scripts/vale-commit-msg.sh":
		tasks["test-hooks tests/devtools"] = true
	case "scripts/cspell-commit-msg.sh":
		tasks["test-hooks tests/skills"] = true
	case "tools/lint-draft.sh", "tools/apm_audit.py", "tools/fix_prose_replacements.py":
		tasks["test-hooks tests/tasks"] = true
	}
}

func selectTestInputs(tasks map[string]bool, file string) {
	if strings.HasSuffix(file, ".py") {
		tasks["test-py"] = true
	}
	dir := "tests"
	if !strings.HasSuffix(file, ".bash") {
		// Helpers are sourced across families. Other inputs belong to tests/<dir>.
		family, _, nested := strings.Cut(strings.TrimPrefix(file, "tests/"), "/")
		if nested {
			dir += "/" + family
		}
	}
	tasks["test-hooks "+dir] = true
}

func selectSkillTests(tasks map[string]bool, file string) {
	tasks["check-generated-skills"] = true
	// Shared library and writer checks cross skill boundaries. Use the
	// existing family rather than inventing a directory for every skill.
	tasks["test-hooks tests/skills"] = true
	// Python tests include the content-posting contract for all source scripts.
	tasks["test-py"] = true
	_, rest, ok := strings.Cut(file, "/skills/")
	if !ok {
		tasks["test-hooks tests/tasks"] = true
		return
	}
	skill, _, _ := strings.Cut(rest, "/")
	switch skill {
	case "fix-prose":
		tasks["test-hooks tests/scripts"] = true
	case "rebase", "resolve-rebase-conflicts":
		tasks["test-hooks tests/tasks"] = true
	case "release", "review-release-version":
		tasks["test-hooks tests/tools"] = true
	}
}
