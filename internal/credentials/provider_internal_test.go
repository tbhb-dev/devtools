// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package credentials

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProvider(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell executable fixtures")
	}
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
	_, err := OPCommand(t.Context(), "key-reference", "", true)
	require.Error(t, err)
	cmd, err := OPCommand(t.Context(), "key-reference", "", false)
	require.NoError(t, err)
	require.Equal(t, []string{"op", "read", "key-reference"}, cmd.Args)
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "existing")
	cmd, err = OPCommand(t.Context(), "key-reference", "", true)
	require.NoError(t, err)
	require.Contains(t, cmd.Env, "OP_SERVICE_ACCOUNT_TOKEN=existing")
	broker := filepath.Join(t.TempDir(), "broker")
	//nolint:gosec // This fixture is an executable owned by the test.
	require.NoError(t, os.WriteFile(broker, []byte("#!/bin/sh\nprintf 'broker-token\\n'\n"), 0o700))
	cmd, err = OPCommand(t.Context(), "key-reference", broker, true)
	require.NoError(t, err)
	require.Contains(t, cmd.Env, "OP_SERVICE_ACCOUNT_TOKEN=broker-token")
	require.NotContains(t, cmd.Env, "OP_SERVICE_ACCOUNT_TOKEN=existing")
	require.NotContains(t, cmd.Args, "broker-token")
	//nolint:gosec // This fixture is an executable owned by the test.
	require.NoError(t, os.WriteFile(broker, []byte("#!/bin/sh\nexit 1\n"), 0o700))
	_, err = OPCommand(t.Context(), "key-reference", broker, true)
	require.Error(t, err)
	bin := t.TempDir()
	op := filepath.Join(bin, "op")
	//nolint:gosec // This fixture is an executable owned by the test.
	require.NoError(
		t,
		os.WriteFile(op, []byte("#!/bin/sh\nprintf '%s:%s:%s' \"$OP_SERVICE_ACCOUNT_TOKEN\" \"$1\" \"$2\"\n"), 0o700),
	)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	data, err := Read(t.Context(), "op", "key-reference", "")
	require.NoError(t, err)
	require.Equal(t, "existing:read:key-reference", string(data))
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
	_, err = Read(t.Context(), "op", "key-reference", "")
	require.Error(t, err)
	path := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(path, []byte("file-key"), 0o600))
	data, err = Read(t.Context(), "file", path, "")
	require.NoError(t, err)
	require.Equal(t, "file-key", string(data))
}
