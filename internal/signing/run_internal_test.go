// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package signing

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("installer targets Unix agents")
	}
	home := t.TempDir()
	var output bytes.Buffer
	t.Setenv("DEVTOOLS_SIGNING_CONFIG", filepath.Join(home, "missing"))
	require.NoError(t, Run(t.Context(), home, []string{"--install"}, nil, &output, &output))
	require.FileExists(t, filepath.Join(home, ".local/bin/ssh-sign"))
	t.Setenv("DEVTOOLS_SIGNING_RUN_DIR", filepath.Join(home, ".local/bin/ssh-sign/invalid"))
	require.Error(t, Run(t.Context(), home, []string{"--invalid-option"}, nil, &output, &output))
	t.Setenv("DEVTOOLS_SIGNING_KEY_TTL", "invalid")
	require.Error(t, Run(t.Context(), home, nil, nil, &output, &output))
	t.Setenv("DEVTOOLS_SIGNING_KEY_TTL", "")
	t.Setenv("PATH", home)
	require.Error(t, Run(t.Context(), home, []string{"--install"}, nil, &output, &output))
	_, err := os.Stat(filepath.Join(home, ".config"))
	require.True(t, os.IsNotExist(err))
}
