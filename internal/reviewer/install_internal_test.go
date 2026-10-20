// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package reviewer

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstall(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	binary := filepath.Join(t.TempDir(), "gh-reviewer")
	require.NoError(t, exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../../cmd/gh-reviewer").Run())
	require.NoError(t, Install(home, binary, false))
	require.NoError(t, Install(home, binary, false))
	target := filepath.Join(home, ".local/bin/gh-reviewer")
	//nolint:gosec // This fixture is an executable owned by the test.
	require.NoError(t, os.WriteFile(target, []byte("old-wrapper"), 0o700))
	require.ErrorIs(t, Install(home, binary, false), os.ErrExist)
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "old-wrapper", string(data))
	require.NoError(t, Install(home, binary, true))
	info, err := os.Stat(target)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
}
