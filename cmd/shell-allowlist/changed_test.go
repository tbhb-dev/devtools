// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//nolint:paralleltest // Tests change the process working directory to select the repository.
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func gitTest(t *testing.T, args ...string) {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "git", args...).CombinedOutput()
	require.NoError(t, err, "%s", out)
}

func TestChangedScripts(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	dir := t.TempDir()
	// Borrow existing history without creating commits or checking out the whole repository.
	gitTest(t, "clone", "--quiet", "--shared", "--no-checkout", root, dir)
	t.Chdir(dir)
	gitTest(t, "read-tree", "HEAD")
	gitTest(t, "update-ref", "refs/remotes/origin/main", "HEAD")
	write := func(path, body string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write(".devtools/shell-allowlist.toml", "commands = ['printf']\n")
	write("packages/agents/source/skills/demo/SKILL.claude.md", "generated skill\n")
	write("packages/agents/source/skills/demo/SKILL.codex.md", "generated skill\n")
	write("tools/fuzz.sh", "printf clean\n")
	write("scripts/clear-commit-agentmsg.sh", "ruby forbidden\nprintf changed\n")
	for _, path := range []string{
		"vendor/bad.sh", "tests/fixtures/bad.bash", "no-extension",
		"packages/agents/claude/.apm/skills/demo/scripts/bad.sh",
		"packages/agents/codex/.apm/skills/demo/scripts/bad.sh",
		".claude/skills/demo/scripts/bad.sh", ".agents/skills/demo/scripts/bad.sh",
	} {
		write(path, "#!/bin/bash\nruby forbidden\n")
	}
	write("new script.bash", "printf new\n")
	gitTest(t, "add", "--", "new script.bash")
	paths, err := changedFiles("origin/main")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"tools/fuzz.sh", "scripts/clear-commit-agentmsg.sh", "new script.bash"}, paths)
	// The checkout deletes the unchanged tracked scripts, and the check must not open them.
	var out bytes.Buffer
	assert.Equal(t, 1, Run([]string{"--changed", "origin/main"}, &out))
	assert.Equal(t, "scripts/clear-commit-agentmsg.sh:1: ruby: not allowed\n", out.String())
	write("scripts/clear-commit-agentmsg.sh", "printf clean\n")
	out.Reset()
	assert.Equal(t, 0, Run([]string{"--changed", "origin/main"}, &out))
	assert.Empty(t, out.String())
	write("untracked.sh", "ruby forbidden\n")
	out.Reset()
	assert.Equal(t, 1, Run([]string{"--changed", "origin/main"}, &out))
	assert.Equal(t, "untracked.sh:1: ruby: not allowed\n", out.String())
	// Repository-local skills have no generator source and remain subject to the gate.
	require.NoError(t, os.Remove("untracked.sh"))
	write(".claude/skills/local/scripts/run.sh", "ruby forbidden\n")
	out.Reset()
	assert.Equal(t, 1, Run([]string{"--changed", "origin/main"}, &out))
	assert.Equal(t, ".claude/skills/local/scripts/run.sh:1: ruby: not allowed\n", out.String())
}

func TestShellAllowlistGateWiring(t *testing.T) {
	t.Chdir("../..")
	data, err := os.ReadFile("mise.toml")
	require.NoError(t, err)
	var config struct {
		Tasks map[string]struct {
			Run any `toml:"run"`
		} `toml:"tasks"`
	}
	require.NoError(t, toml.Unmarshal(data, &config))
	assert.Contains(t, config.Tasks["lint"].Run, "mise run lint-shell-allowlist")
	assert.Equal(t, "go run ./cmd/shell-allowlist --changed origin/main", config.Tasks["lint-shell-allowlist"].Run)

	data, err = os.ReadFile(".github/workflows/shell-allowlist.yml")
	require.NoError(t, err)
	var workflow struct {
		On   map[string]any `yaml:"on"`
		Jobs map[string]struct {
			Name  string `yaml:"name"`
			If    string `yaml:"if"`
			Steps []struct {
				If   string            `yaml:"if"`
				Run  string            `yaml:"run"`
				With map[string]any    `yaml:"with"`
				Env  map[string]string `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(data, &workflow))
	assert.Contains(t, workflow.On, "pull_request")
	assert.Nil(t, workflow.On["pull_request"], "every PR must report the job")
	job := workflow.Jobs["shell-allowlist"]
	assert.Equal(t, "shell-allowlist", job.Name)
	assert.Empty(t, job.If)
	var checked, history bool
	for _, step := range job.Steps {
		if step.With["fetch-depth"] == 0 {
			history = true
		}
		if step.Run == `go run ./cmd/shell-allowlist --changed "$SHELL_ALLOWLIST_BASE"` {
			checked = true
			assert.Empty(t, step.If)
			assert.Equal(t, "${{ github.event.pull_request.base.sha }}", step.Env["SHELL_ALLOWLIST_BASE"])
		}
	}
	assert.True(t, history, "merge-base selection needs history")
	assert.True(t, checked, "the analyzer must run even with no changed scripts")
}
