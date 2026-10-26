// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestContextStateDirectoryKeepsGitSpelling(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	data, err := exec.CommandContext(t.Context(), "git", "init", "--quiet").CombinedOutput()
	require.NoError(t, err, "%s", data)
	gitDir, err := exec.CommandContext(t.Context(), "git", "rev-parse", "--absolute-git-dir").Output()
	require.NoError(t, err)
	var out, stderr bytes.Buffer
	require.Equal(t, 0, agentrepo.Run([]string{"context", "state-dir"}, nil, &out, &stderr))
	require.Empty(t, stderr.String())
	// Keep Git's forward slashes on Windows, as the shell does.
	require.Equal(t, strings.TrimRight(string(gitDir), "\n")+"/agent-skills\n", out.String())
	require.DirExists(t, strings.TrimSuffix(out.String(), "\n"))
}

func TestContextLiteralShellPaths(t *testing.T) {
	t.Setenv("HOME", "native-home")
	for _, tc := range []struct{ argument, want string }{
		{"~/docs", "/c/shell home/docs\n"},
		{"/literal/../path", "/literal/../path\n"},
	} {
		var out, stderr bytes.Buffer
		require.Equal(
			t,
			0,
			agentrepo.Run([]string{"context", "--home", "/c/shell home", "path", tc.argument}, nil, &out, &stderr),
		)
		require.Equal(t, tc.want, out.String())
		require.Empty(t, stderr.String())
	}
}
