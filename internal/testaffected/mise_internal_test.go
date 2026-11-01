// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package testaffected

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
)

// TestTaskProcess is the portable subprocess used by the fixture mise tasks.
func TestTaskProcess(t *testing.T) {
	t.Parallel()
	if os.Getenv("TEST_AFFECTED_PROCESS") != "1" {
		return
	}
	task := os.Args[len(os.Args)-1]
	if task == "selector" {
		os.Exit(Run(t.Context(), os.Stdout, os.Stderr))
	}
	file, err := os.OpenFile("task.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(2)
	}
	_, err = fmt.Fprintln(file, task)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		os.Exit(2)
	}
	if task == os.Getenv("TEST_AFFECTED_FAIL") {
		os.Exit(1)
	}
	os.Exit(0)
}

type taskDefinition struct {
	Run any `toml:"run"`
}

func miseFixture(t *testing.T) string {
	t.Helper()
	dir := fixture(t)
	data, err := os.ReadFile("../../mise.toml")
	require.NoError(t, err)
	var config struct {
		Tasks map[string]taskDefinition `toml:"tasks"`
	}
	require.NoError(t, toml.Unmarshal(data, &config))
	binary, err := os.Executable()
	require.NoError(t, err)
	helper := func(task string) string {
		return fmt.Sprintf("%q -test.run=^TestTaskProcess$ -- %s", filepath.ToSlash(binary), task)
	}
	test := config.Tasks["test"]
	if test.Run == "go run ./cmd/test-affected" {
		test.Run = helper("selector")
	}
	tasks := map[string]taskDefinition{"test": test, "test-full": config.Tasks["test-full"]}
	for _, task := range []string{"check-generated-skills", "test-go", "test-py"} {
		tasks[task] = taskDefinition{Run: helper(task)}
	}
	body, err := toml.Marshal(struct {
		Tasks map[string]taskDefinition `toml:"tasks"`
	}{Tasks: tasks})
	require.NoError(t, err)
	write(t, dir, "mise.toml", string(body))
	write(t, dir, ".gitignore", "task.log\nPR_AGENTDESC.md\n")
	git(t, dir, "add", "mise.toml", ".gitignore")
	git(t, dir, "commit", "-qm", "Fixture tasks")
	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	cmd := exec.CommandContext(t.Context(), "mise", "trust", "mise.toml")
	cmd.Dir = dir
	cmd.Env = fixtureEnv()
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	t.Cleanup(func() {
		// Remove the trust record with the temporary configuration.
		untrust := exec.CommandContext(context.WithoutCancel(t.Context()), "mise", "untrust", "mise.toml")
		untrust.Dir = dir
		untrust.Env = fixtureEnv()
		require.NoError(t, untrust.Run())
	})
	return dir
}

func runMise(t *testing.T, dir, task, fail string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "mise", "run", task)
	cmd.Dir = dir
	cmd.Env = append(fixtureEnv(), "TEST_AFFECTED_PROCESS=1", "TEST_AFFECTED_FAIL="+fail)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func taskLog(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "task.log"))
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(data)
}

func TestMiseGateContexts(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"staged", "committed", "release", "empty", "unknown", "infrastructure"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			dir := miseFixture(t)
			path, want := "internal/buildmeta/buildmeta_test.go", "test-go\n"
			switch mode {
			case "release":
				write(t, dir, "packages/devtools/src/devtools/other.py", "change")
				git(t, dir, "add", "packages/devtools/src/devtools/other.py")
				git(t, dir, "commit", "-qm", "Before release")
				git(t, dir, "update-ref", "refs/remotes/origin/release", "HEAD")
				git(t, dir, "branch", "release", "HEAD~1")
				write(t, dir, "PR_AGENTDESC.md", "---\nbase: release\n---\n# Draft\n")
			case "empty":
				path, want = "docs/guide.md", ""
			case "unknown":
				path = "unknown.file"
			case "infrastructure":
				path = "internal/testaffected/select.go"
			}
			if mode == "unknown" || mode == "infrastructure" {
				want = "check-generated-skills\ntest-go\ntest-py\n"
			}
			write(t, dir, path, "change")
			git(t, dir, "add", path)
			if mode == "committed" || mode == "release" {
				git(t, dir, "commit", "-qm", "Change")
			}
			output, err := runMise(t, dir, "test", "")
			require.NoError(t, err, "%s", output)
			require.Equal(t, want, taskLog(t, dir), "%s", output)
			message := "Selected suites:"
			if want == "" {
				message = "No affected tests selected"
			}
			require.Contains(t, output, message)
		})
	}
}

func TestMiseFailureAndFullReproducer(t *testing.T) {
	t.Parallel()
	dir := miseFixture(t)
	write(t, dir, "internal/buildmeta/buildmeta_test.go", "change")
	git(t, dir, "add", "internal/buildmeta/buildmeta_test.go")
	output, err := runMise(t, dir, "test", "test-py")
	require.NoError(t, err, "%s", output)
	require.Equal(t, "test-go\n", taskLog(t, dir))
	output, err = runMise(t, dir, "test-full", "test-py")
	require.Error(t, err, "%s", output)
	require.True(t, strings.HasSuffix(taskLog(t, dir), "check-generated-skills\ntest-go\ntest-py\n"))
	output, err = runMise(t, dir, "test", "test-go")
	require.Error(t, err, "%s", output)
	write(t, dir, "mise.toml", "[tasks.test]\nrun = 'missing-selector-command'\n")
	output, err = runMise(t, dir, "test", "")
	require.Error(t, err, "%s", output)
}

func TestFrozenGateWrapperStopsOnSelectedFailure(t *testing.T) {
	t.Parallel()
	python, err := exec.LookPath("python3")
	if err != nil {
		python, err = exec.LookPath("python")
	}
	require.NoError(t, err)
	gates, err := filepath.Abs("../../packages/agents/source/lib/gates.py")
	require.NoError(t, err)
	dir := miseFixture(t)
	write(t, dir, "internal/buildmeta/buildmeta_test.go", "change")
	git(t, dir, "add", "internal/buildmeta/buildmeta_test.go")
	for _, fail := range []string{"test-go", ""} {
		cmd := exec.CommandContext(t.Context(), python, gates, "test", "", "true", "mise", "run", "test")
		cmd.Dir = dir
		cmd.Env = append(fixtureEnv(), "TEST_AFFECTED_PROCESS=1", "TEST_AFFECTED_FAIL="+fail)
		output, runErr := cmd.CombinedOutput()
		if fail == "" {
			require.NoError(t, runErr, "%s", output)
		} else {
			require.Error(t, runErr, "%s", output)
		}
	}
	require.Equal(t, strings.Repeat("test-go\n", 2), taskLog(t, dir))
}
