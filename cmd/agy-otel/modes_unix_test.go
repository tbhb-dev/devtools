// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//go:build !windows

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModes(t *testing.T) {
	t.Parallel()
	binary := buildHook(t)
	state := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "sh", "-c", `umask 000; exec "$@"`, "sh", binary, "hook", "Stop")
	cmd.Env = append(os.Environ(), "XDG_STATE_HOME="+state)
	cmd.Stdin = bytes.NewBufferString(`{}`)
	output, commandErr := cmd.CombinedOutput()
	require.NoError(t, commandErr)
	require.Empty(t, output)
	for _, name := range []string{"agy-otel", "agy-otel/tmp", "agy-otel/new"} {
		info, err := os.Stat(filepath.Join(state, name))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
	list := entries(t, state, "new")
	require.Len(t, list, 1)
	assert.Regexp(t, `^[0-9]+\.[0-9]+\.[A-Z2-7]+\.json$`, list[0].Name())
	info, err := list[0].Info()
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
