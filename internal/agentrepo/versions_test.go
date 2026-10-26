// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestVersionCheckCommand(t *testing.T) {
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "missing"))
	for _, args := range [][]string{
		{"check-versions"},
		{"check-versions", "v1.2.3"},
	} {
		var out, stderr bytes.Buffer
		require.Equal(t, 1, agentrepo.Run(args, nil, &out, &stderr))
		require.Empty(t, out.String())
		require.Contains(t, stderr.String(), "check-versions: git rev-parse")
	}
}

func TestVersionCheckFailingGit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "git")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	source := filepath.Join(root, "git.go")
	require.NoError(
		t,
		os.WriteFile(source, []byte("package main\nimport \"os\"\nfunc main() { os.Exit(42) }\n"), 0o600),
	)
	build := exec.CommandContext(t.Context(), "go", "build", "-o", path, source)
	data, err := build.CombinedOutput()
	require.NoError(t, err, "%s", data)
	t.Setenv("PATH", root)
	var out, stderr bytes.Buffer
	require.Equal(t, 1, agentrepo.Run([]string{"check-versions", "1.2.3"}, nil, &out, &stderr))
	require.Empty(t, out.String())
	require.Contains(t, stderr.String(), "exit status 42", "AC8: failing Git read")
}
