// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWatchProbeOffline(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "agentrepo.exe")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "./cmd/agentrepo")
	build.Dir = "../.."
	output, err := build.CombinedOutput()
	require.NoError(t, err, "%s", output)
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", "")
	var out, stderr bytes.Buffer
	probe := exec.CommandContext(t.Context(), binary, "checks", "watch", "--help")
	probe.Stdout, probe.Stderr = &out, &stderr
	require.NoError(t, probe.Run())
	require.Contains(t, out.String(), "agentrepo checks watch [pull-request-number]")
	require.Empty(t, stderr.String())
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, files)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.Equal(t, dir, cwd)
	require.NotContains(t, out.String(), "WATCHING")
}
