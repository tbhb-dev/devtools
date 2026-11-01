//go:build darwin || linux

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package testenv

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func executable(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o600))
	info, err := os.Stat(path)
	require.NoError(t, err)
	const ownerExecute = 0o100
	require.NoError(t, os.Chmod(path, info.Mode()|ownerExecute))
	info, err = os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestEnvironment(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cargo := filepath.Join(dir, "cargo")
	require.NoError(t, os.MkdirAll(filepath.Join(cargo, "bin"), 0o700))
	stores := map[string]string{
		"MISE_DATA_DIR": "/store/mise", "MISE_CACHE_DIR": "/cache/mise",
		"CARGO_HOME": cargo, "RUSTUP_HOME": "/store/rustup",
		"GOMODCACHE": "/cache/modules", "GOCACHE": "/cache/go",
	}
	inherited := []string{
		"HOME=/ambient", "PATH=/fake", "TERM=xterm", "TMPDIR=/tmp", "CI=true",
		"GITHUB_ACTIONS=true", "DEVTOOLS_EXAMPLE=a=b", "SECRET=hidden",
		"GIT_CONFIG_COUNT=1", "MISE_CONFIG_FILE=/ambient.toml", "XDG_CONFIG_HOME=/ambient",
		"RUSTUP_TOOLCHAIN=host", "BASH_ENV=/ambient.sh",
	}
	env := envMap(
		environment(
			inherited,
			stores,
			"/repo",
			"/temporary",
			"/temporary/config.toml",
			"/mise/bin/mise",
			[]string{"/tools/first", "/tools/second"},
		),
	)
	require.Equal(t, map[string]string{
		"HOME": "/temporary",
		"PATH": stockPath + ":/repo/bin:/mise/bin:/tools/first:/tools/second:" + filepath.Join(
			cargo,
			"bin",
		),
		"GIT_CONFIG_GLOBAL":       "/dev/null",
		"GIT_CONFIG_NOSYSTEM":     "1",
		"MISE_GLOBAL_CONFIG_FILE": "/temporary/config.toml",
		"MISE_CEILING_PATHS":      "/",
		"TERM":                    "xterm",
		"TMPDIR":                  "/tmp",
		"CI":                      "true",
		"GITHUB_ACTIONS":          "true",
		"DEVTOOLS_EXAMPLE":        "a=b",
		"MISE_DATA_DIR":           "/store/mise",
		"MISE_CACHE_DIR":          "/cache/mise",
		"CARGO_HOME":              cargo,
		"RUSTUP_HOME":             "/store/rustup",
		"GOMODCACHE":              "/cache/modules",
		"GOCACHE":                 "/cache/go",
	}, env)
	delete(stores, "CARGO_HOME")
	require.Equal(
		t,
		stockPath+":/repo/bin:/mise/bin",
		envMap(environment(nil, stores, "/repo", dir, "config.toml", "/mise/bin/mise", nil))["PATH"],
	)
}

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	executable(t, filepath.Join(dir, "git"), "printf '%s\\n' '"+dir+"'")
	executable(t, filepath.Join(dir, "go"), "printf '/modules\\n/gocache\\n'")
	executable(t, filepath.Join(dir, "mise"), `if [ "$1" = trust ]; then exit 0; fi
test "$1" = bin-paths || exit 80
test -f "$MISE_GLOBAL_CONFIG_FILE" || exit 81
test ! -s "$MISE_GLOBAL_CONFIG_FILE" || exit 82
test "$GIT_CONFIG_GLOBAL" = /dev/null || exit 83
test -z "${BASH_ENV:-}" || exit 84
printf '%s\n' "$DEVTOOLS_FIXTURE/bin"`)
	t.Setenv("DEVTOOLS_FIXTURE", dir)
	return dir
}

func TestRunUsesStockSedAndCleansHome(t *testing.T) {
	dir := fixture(t)
	t.Setenv("PATH", dir)
	executable(t, filepath.Join(dir, "sed"), "exit 99")
	var stdout, stderr bytes.Buffer
	code := Run(
		[]string{"/bin/sh", "-c", `command -v sed; printf '%s\n' "$HOME"; sed 's/a/b/'; test -z "${SECRET:-}"`},
		strings.NewReader("a\n"),
		&stdout,
		&stderr,
	)
	require.Equal(t, 0, code, stderr.String())
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	require.Contains(t, []string{"/usr/bin/sed", "/bin/sed"}, lines[0])
	require.Equal(t, "b", lines[2])
	_, err := os.Stat(lines[1])
	require.True(t, os.IsNotExist(err), "temporary HOME should be removed")
}

func TestRunResolvesChildPathAndExit(t *testing.T) {
	dir := fixture(t)
	t.Setenv("PATH", dir)
	executable(t, filepath.Join(dir, "bin", "fixture-command"), "echo child; exit 7")
	var stdout, stderr bytes.Buffer
	require.Equal(t, 7, Run([]string{"fixture-command"}, nil, &stdout, &stderr), stderr.String())
	require.Equal(t, "child\n", stdout.String())
}

func TestRunFailures(t *testing.T) {
	t.Setenv("PATH", fixture(t))
	for _, tc := range []struct {
		name   string
		args   []string
		broken string
		want   int
	}{
		{"usage", nil, "", 2},
		{"repository", []string{"true"}, "git", 9},
		{"stores", []string{"true"}, "go", 9},
		{"mise", []string{"true"}, "mise", 9},
		{"command", []string{"missing-testenv-command"}, "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixture(t)
			t.Setenv("PATH", dir)
			if tc.broken != "" {
				executable(t, filepath.Join(dir, tc.broken), "exit 9")
			}
			var stderr bytes.Buffer
			require.Equal(t, tc.want, Run(tc.args, nil, &bytes.Buffer{}, &stderr), stderr.String())
		})
	}
}

func TestToolStores(t *testing.T) {
	t.Setenv("PATH", fixture(t))
	values := make([]string, 0, 8)
	values = append(values, "HOME=/original", "XDG_DATA_HOME=/data")
	stores, err := toolStores(values)
	require.NoError(t, err)
	require.Equal(t, "/data/mise", stores["MISE_DATA_DIR"])
	require.Equal(t, "/original/.cargo", stores["CARGO_HOME"])
	require.Equal(t, "/original/.rustup", stores["RUSTUP_HOME"])
	require.Equal(t, "/modules", stores["GOMODCACHE"])
	require.Equal(t, "/gocache", stores["GOCACHE"])
	for key := range stores {
		values = append(values, key+"=/explicit/"+key)
	}
	stores, err = toolStores(values)
	require.NoError(t, err)
	for key, value := range stores {
		require.Equal(t, "/explicit/"+key, value)
	}
}
