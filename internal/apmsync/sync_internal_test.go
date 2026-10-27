// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package apmsync

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReconcile(t *testing.T) {
	t.Parallel()
	for _, document := range []struct{ path, key string }{
		{".claude/settings.json", "hooks"}, {".agents/hooks.json", "apm"}, {".codex/hooks.json", "hooks"},
	} {
		t.Run(document.path, func(t *testing.T) {
			t.Parallel()
			for _, fixture := range []struct{ name, input, want string }{
				{"siblings", `{"KEY":{},"local":9007199254740993}`, `{"local":9007199254740993}`},
				{"owned", `{"KEY":{}}`, ""},
				{"unowned", `{"local":true}`, `{"local":true}`},
				{"empty", `{}`, ""},
				{"zero", "", ""},
				{"whitespace", " \t\r\n", ""},
				{"missing", "", ""},
			} {
				t.Run(fixture.name, func(t *testing.T) {
					t.Parallel()
					checkFixture(t, document.path, document.key, fixture.input, fixture.want, fixture.name != "missing")
				})
			}
		})
	}
}

func checkFixture(t *testing.T, document, key, input, want string, exists bool) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, document)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	if exists {
		require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(input, "KEY", key)), 0o600))
	}
	for _, harness := range []string{".claude", ".agents", ".codex"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, harness), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(root, harness, "apm-hooks.json"), nil, 0o600))
	}
	for range 2 {
		require.NoError(t, reconcile(root))
		if want == "" {
			require.NoFileExists(t, path)
		} else {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.JSONEq(t, want, string(data))
		}
		for _, harness := range []string{".claude", ".agents", ".codex"} {
			require.NoFileExists(t, filepath.Join(root, harness, "apm-hooks.json"))
		}
	}
}

func TestCleanupFailureRetainsEarlierEffects(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".agents"), 0o750))
	first := filepath.Join(root, ".claude/settings.json")
	second := filepath.Join(root, ".agents/hooks.json")
	require.NoError(t, os.WriteFile(first, []byte(`{"hooks":{}}`), 0o600))
	require.NoError(t, os.WriteFile(second, []byte(`{broken`), 0o600))
	require.Error(t, reconcile(root))
	require.NoFileExists(t, first)
	data, err := os.ReadFile(second)
	require.NoError(t, err)
	require.Equal(t, `{broken`, string(data))
}

func TestCleanupWriteAndSidecarFailures(t *testing.T) {
	t.Parallel()
	for _, blocked := range []string{".claude/settings.json.tmp", ".agents/apm-hooks.json"} {
		t.Run(blocked, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(root, blocked), 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(root, blocked, "child"), nil, 0o600))
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), 0o750))
			require.NoError(
				t,
				os.WriteFile(filepath.Join(root, ".claude/settings.json"), []byte(`{"hooks":{}}`), 0o600),
			)
			require.Error(t, reconcile(root))
		})
	}
}

// The test executable also runs as a portable APM process fixture.
func TestMain(m *testing.M) {
	if os.Getenv("APMSYNC_HELPER") == "1" {
		if _, err := os.Stat(".claude/apm-hooks.json"); !os.IsNotExist(err) {
			os.Exit(91)
		}
		if _, err := os.Stat(".claude/settings.json"); !os.IsNotExist(err) {
			os.Exit(92)
		}
		if err := os.WriteFile("installed", []byte("install\n"), 0o600); err != nil {
			os.Exit(93)
		}
		fmt.Fprintln(os.Stdout, "deployment output")
		fmt.Fprintln(os.Stderr, "deployment diagnostic")
		if os.Getenv("APMSYNC_FAIL") == "1" {
			os.Exit(23)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestRunInstallStatusAndStreams(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(strconv.FormatBool(fail), func(t *testing.T) {
			root := t.TempDir()
			binary, err := os.Executable()
			require.NoError(t, err)
			bin := t.TempDir()
			name := "apm"
			if runtime.GOOS == "windows" {
				name += ".exe"
			}
			require.NoError(t, os.Link(binary, filepath.Join(bin, name)))
			t.Setenv("PATH", bin)
			t.Setenv("APMSYNC_HELPER", "1")
			t.Setenv("APMSYNC_FAIL", map[bool]string{true: "1", false: "0"}[fail])
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), 0o750))
			require.NoError(
				t,
				os.WriteFile(filepath.Join(root, ".claude/settings.json"), []byte(`{"hooks":{}}`), 0o600),
			)
			require.NoError(t, os.WriteFile(filepath.Join(root, ".claude/apm-hooks.json"), nil, 0o600))
			var out, stderr bytes.Buffer
			want := 0
			if fail {
				want = 23
			}
			require.Equal(t, want, Run(t.Context(), root, nil, &out, &stderr))
			require.Equal(t, "deployment output\n", out.String())
			require.Equal(t, "deployment diagnostic\n", stderr.String())
			require.FileExists(t, filepath.Join(root, "installed"))
			require.NoFileExists(t, filepath.Join(root, ".claude/settings.json"))
		})
	}
}

func TestRunStopsBeforeInstallOnInspectionFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".claude/settings.json"), []byte(`{broken`), 0o600))
	var out, stderr bytes.Buffer
	require.Equal(t, 4, Run(t.Context(), root, nil, &out, &stderr))
	require.Empty(t, out.String())
	require.Contains(t, stderr.String(), "settings.json")
	require.NoFileExists(t, filepath.Join(root, "installed"))
}

func TestMissingInstaller(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	var out, stderr bytes.Buffer
	require.Equal(t, 127, Run(t.Context(), root, nil, &out, &stderr))
	require.Empty(t, out.String())
	require.Contains(t, stderr.String(), "apm")
}

func TestInvalidDocument(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), 0o750))
	path := filepath.Join(root, ".claude/settings.json")
	require.NoError(t, os.WriteFile(path, []byte(`[1,2]`), 0o600))
	var out, stderr bytes.Buffer
	require.Equal(t, 5, Run(t.Context(), root, nil, &out, &stderr))
	require.Empty(t, out.String())
	require.NotEmpty(t, stderr.String())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, `[1,2]`, string(data))
}

func TestSidecarFailureStillRemovesOtherSidecars(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude/apm-hooks.json"), 0o750))
	for _, harness := range []string{".agents", ".codex"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, harness), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(root, harness, "apm-hooks.json"), nil, 0o600))
	}
	require.Error(t, reconcile(root))
	for _, harness := range []string{".agents", ".codex"} {
		require.NoFileExists(t, filepath.Join(root, harness, "apm-hooks.json"))
	}
}
