// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//nolint:paralleltest // Tests select a repository with t.Chdir.
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestSummary(t *testing.T) {
	t.Chdir(t.TempDir())
	gitTest(t, "init", "--quiet")
	require.NoError(t, os.MkdirAll(".devtools", 0o700))
	require.NoError(t, os.WriteFile(".devtools/shell-allowlist.toml", []byte("commands = ['printf']\n"), 0o600))
	for path, body := range map[string]string{
		"a script.sh":    "ruby secret-argument\n$cmd\nprintf ok\n",
		"clean.bash":     "printf clean\n",
		"broken.sh":      "if secret-source\n",
		"ignored.txt":    "ruby\n",
		"vendor/copy.sh": "ruby\n",
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	gitTest(t, "add", "--", "a script.sh")
	var out bytes.Buffer
	assert.Zero(t, Run([]string{"--summary"}, &out))
	assert.Equal(
		t,
		"## Shell burn-down\n\nTotal: 3 violations across 4 scripts; 1 analysis errors.\n\n| Script | Violations | Analysis errors |\n| --- | ---: | ---: |\n| a script.sh | 2 | 0 |\n| broken.sh | 0 | 1 |\n| clean.bash | 0 | 0 |\n| vendor/copy.sh | 1 | 0 |\n",
		out.String(),
	)
	assert.NotContains(t, out.String(), "secret")
}

func TestSummaryErrorsAreNonGating(t *testing.T) {
	t.Chdir(t.TempDir())
	var out bytes.Buffer
	assert.Zero(t, Run([]string{"--summary"}, &out))
	assert.Contains(t, out.String(), "Shell burn-down")
	assert.Contains(t, out.String(), "git ls-files")
	gitTest(t, "init", "--quiet")
	out.Reset()
	assert.Zero(t, Run([]string{"--summary"}, &out))
	assert.Contains(t, out.String(), "shell-allowlist.toml")
}

func TestConsumerPayload(t *testing.T) {
	t.Chdir("../..")
	data, err := os.ReadFile(".devtools/vendir.yml")
	require.NoError(t, err)
	var config struct {
		Directories []struct {
			Path     string `yaml:"path"`
			Contents []struct {
				IncludePaths []string `yaml:"includePaths"`
			} `yaml:"contents"`
		} `yaml:"directories"`
	}
	require.NoError(t, yaml.Unmarshal(data, &config))
	shipped := false
	for _, dir := range config.Directories {
		if dir.Path == ".devtools" {
			for _, content := range dir.Contents {
				shipped = shipped || assert.Contains(t, content.IncludePaths, ".devtools/**")
			}
		}
	}
	require.True(t, shipped)
	require.FileExists(t, ".devtools/shell-allowlist.toml")
	data, err = os.ReadFile(".devtools/tasks/lint.toml")
	require.NoError(t, err)
	var tasks map[string]struct {
		Run any `toml:"run"`
	}
	require.NoError(t, toml.Unmarshal(data, &tasks))
	assert.Equal(t, "shell-allowlist --changed origin/main", tasks["devtools:lint-shell-allowlist"].Run)
	data, err = os.ReadFile("README.md")
	require.NoError(t, err)
	assert.Contains(t, string(data), "- `shell-allowlist`")
	assert.Contains(t, string(data), "go install github.com/tbhb-dev/devtools/cmd/shell-allowlist@latest")
	assert.Contains(t, string(data), "mise run devtools:lint-shell-allowlist")
}

func TestSummaryIncludesBats(t *testing.T) {
	t.Chdir(t.TempDir())
	gitTest(t, "init", "--quiet")
	require.NoError(t, os.Mkdir(".devtools", 0o700))
	require.NoError(t, os.WriteFile(".devtools/shell-allowlist.toml", []byte("commands = ['printf']\n"), 0o600))
	require.NoError(t, os.WriteFile("suite.bats", []byte("@test 'sample' { printf ok; }\n"), 0o600))
	var out bytes.Buffer
	assert.Zero(t, Run([]string{"--summary"}, &out))
	assert.Contains(t, out.String(), "Total: 0 violations across 1 scripts; 1 analysis errors.")
	assert.Contains(t, out.String(), "| suite.bats | 0 | 1 |")
}
