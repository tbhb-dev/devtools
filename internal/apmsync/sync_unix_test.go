//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package apmsync_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/apmsync"
)

func TestUnreadableSettingsStopInstall(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root can read mode-zero settings")
	}
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), 0o750))
	path := filepath.Join(root, ".claude/settings.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"hooks":{}}`), 0o000))
	var out, stderr bytes.Buffer
	require.Equal(t, 2, apmsync.Run(t.Context(), root, nil, &out, &stderr))
	require.Empty(t, out.String())
	require.Contains(t, stderr.String(), "permission denied")
	require.FileExists(t, path)
	require.NoFileExists(t, filepath.Join(root, "installed"))
}
