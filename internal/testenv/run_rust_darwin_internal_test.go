//go:build darwin && integration

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package testenv

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRunIgnoresGlobalRustPin needs real mise, Bats and an installed Rust toolchain.
// Run with go test -tags=integration ./internal/testenv -run TestRunIgnoresGlobalRustPin -v.
func TestRunIgnoresGlobalRustPin(t *testing.T) {
	mise, err := exec.LookPath("mise")
	require.NoError(t, err)
	mise, err = filepath.EvalSymlinks(mise)
	require.NoError(t, err)
	stores, err := toolStores(os.Environ())
	require.NoError(t, err)
	for key, value := range stores {
		t.Setenv(key, value)
	}
	installed := exec.CommandContext(t.Context(), mise, "latest", "--installed", "rust")
	output, err := installed.CombinedOutput()
	require.NoError(t, err, string(output))
	version := strings.TrimSpace(string(output))
	require.NotEmpty(t, version)
	home := t.TempDir()
	config := filepath.Join(home, ".config", "mise", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(config), 0o700))
	require.NoError(t, os.WriteFile(config, []byte("[tools]\nrust = \""+version+"\"\n"), 0o600))
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("MISE_GLOBAL_CONFIG_FILE", config)
	t.Setenv("MISE_CONFIG_FILE", "")
	t.Setenv("MISE_CEILING_PATHS", filepath.Dir(home))
	pinned := exec.CommandContext(t.Context(), mise, "which", "rustc")
	pinned.Dir = home
	output, err = pinned.CombinedOutput()
	require.NoError(t, err, string(output))
	rustc := strings.TrimSpace(string(output))
	require.FileExists(t, rustc)
	store := exec.CommandContext(t.Context(), mise, "where", "rust@"+version)
	store.Dir = home
	output, err = store.CombinedOutput()
	require.NoError(t, err, string(output))
	rustStore := strings.TrimSpace(string(output))
	resolvedStore, err := filepath.EvalSymlinks(rustStore)
	require.NoError(t, err)
	require.DirExists(t, resolvedStore)

	t.Chdir(filepath.Join("..", ".."))
	var stdout, stderr bytes.Buffer
	code := Run([]string{"/bin/sh", "-ec", `
test ! -s "$MISE_GLOBAL_CONFIG_FILE"
test "$MISE_GLOBAL_CONFIG_FILE" != "$1"
test "$MISE_DATA_DIR" = "$2"
test "$(mise ls --current rust --json)" = "[]"
mise where "rust@$3"
bats --filter 'Rust task commands preserve workspace, lock and argument wiring|fuzz tasks require a nightly and run targets from the fuzz directory' tests/tasks/rust.bats
`, "rust-pin", config, stores["MISE_DATA_DIR"], version}, nil, &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String()+stdout.String())
	require.Contains(t, stdout.String(), rustStore)
	require.Contains(t, stdout.String(), "1..2")
	require.Contains(t, stdout.String(), "ok 1 Rust task commands preserve workspace, lock and argument wiring")
	require.Contains(t, stdout.String(), "ok 2 fuzz tasks require a nightly and run targets from the fuzz directory")
	t.Logf("global Rust %s resolved %s; isolated configuration ignored the pin and retained the store", version, rustc)
}
