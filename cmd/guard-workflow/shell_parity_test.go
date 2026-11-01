//go:build darwin || linux

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The portable tables run on every platform. Where the frozen scripts
// run natively, also compare their actual protocol with the built binary.
func TestAC9FrozenShell(t *testing.T) {
	t.Parallel()
	binary := filepath.Join(t.TempDir(), "guard-workflow")
	goTool, err := exec.LookPath("go")
	require.NoError(t, err)
	build := exec.CommandContext(t.Context(), goTool, "build", "-o", binary, ".")
	output, err := build.CombinedOutput()
	require.NoError(t, err, "%s", output)
	bash, err := exec.LookPath("bash")
	require.NoError(t, err)
	scripts := map[string]string{
		"pr": "guard-gh.sh", "merge-pr": "guard-merge.sh",
		"watch-pr": "guard-watch.sh", "fix-pr": "guard-fix.sh",
	}
	for _, tc := range commandCases() {
		t.Run(tc.ac+"/"+tc.skill+"/"+tc.command, func(t *testing.T) {
			t.Parallel()
			input := payload(t, "Bash", tc.command)
			script := filepath.Join("..", "..", "packages", "agents", "source", "skills",
				tc.skill, "claude", "scripts", scripts[tc.skill])
			want := processResult(t, input, bash, script)
			got := processResult(t, input, binary, tc.skill)
			assert.Equal(t, want, got)
			assert.Empty(t, got.stdout)
			if tc.blocked {
				assert.Equal(t, 2, want.code)
			} else {
				assert.Equal(t, result{}, want)
			}
		})
	}
}

// Covers payloads with text after the first JSON value, which the shell guard rejects.
func TestAC8TrailingInputFrozenShell(t *testing.T) {
	t.Parallel()
	binary := filepath.Join(t.TempDir(), "guard-workflow")
	goTool, err := exec.LookPath("go")
	require.NoError(t, err)
	build := exec.CommandContext(t.Context(), goTool, "build", "-o", binary, ".")
	output, err := build.CombinedOutput()
	require.NoError(t, err, "%s", output)
	bash, err := exec.LookPath("bash")
	require.NoError(t, err)
	scripts := map[string]string{
		"pr": "guard-gh.sh", "merge-pr": "guard-merge.sh",
		"watch-pr": "guard-watch.sh", "fix-pr": "guard-fix.sh",
	}
	for skill, name := range scripts {
		input := payload(t, "Bash", "gh pr merge 1; gh pr checks --watch") + "x"
		script := filepath.Join("..", "..", "packages", "agents", "source", "skills",
			skill, "claude", "scripts", name)
		assert.NotEqual(t, 2, processResult(t, input, bash, script).code, skill)
		assert.Equal(t, result{}, processResult(t, input, binary, skill), skill)
		assert.Equal(t, result{}, execute(input, skill), skill)
	}
}

func processResult(t *testing.T, input, binary string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	command := exec.CommandContext(t.Context(), binary, args...)
	command.Stdin, command.Stdout, command.Stderr = strings.NewReader(input), &out, &errOut
	var code int
	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		code = exitErr.ExitCode()
	}
	return result{code, out.String(), errOut.String()}
}
