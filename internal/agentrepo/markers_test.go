// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func markerRepo(t *testing.T) {
	t.Helper()
	// Match the isolated shell fixtures: inherited Git state mustn't redirect Git.
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(key, "GIT_") {
			t.Setenv(key, "")
			require.NoError(t, os.Unsetenv(key))
		}
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_ATTR_NOSYSTEM", "1")
	markerGit(t, "init", "--quiet")
}

func markerGit(t *testing.T, args ...string) {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "git", args...).CombinedOutput()
	require.NoError(t, err, "%s", out)
}

func markerWrite(t *testing.T, path, data string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
}

func markerRun(t *testing.T, staged bool, paths ...string) (int, string) {
	t.Helper()
	args := []string{"markers", "scan"}
	if staged {
		args = append(args, "--staged")
	}
	var out, stderr bytes.Buffer
	code := agentrepo.Run(append(args, paths...), nil, &out, &stderr)
	require.Empty(t, stderr.String())
	return code, out.String()
}

func TestMarkerContentSources(t *testing.T) {
	t.Setenv("GIT_ATTR_NOSYSTEM", "1")
	t.Chdir(t.TempDir())
	markerRepo(t)
	markerWrite(t, "index", "<<<<<<< staged\n")
	markerWrite(t, "worktree", "clean\n")
	markerGit(t, "add", "--", "index", "worktree")
	markerWrite(t, "index", "clean\n")
	markerWrite(t, "worktree", ">>>>>>> worktree\n|||||||\n")
	markerWrite(t, "missing-index", "<<<<<<< untracked\n")
	require.NoError(t, os.Mkdir("directory", 0o700))
	paths := []string{"index", "worktree", "missing-index", "directory", "absent"}
	code, output := markerRun(t, true, paths...)
	require.Equal(t, 1, code)
	require.Equal(t, "index  1 marker line(s)\n", output)
	code, output = markerRun(t, false, paths...)
	require.Equal(t, 1, code)
	require.Equal(t, "worktree  2 marker line(s)\nmissing-index  1 marker line(s)\n", output)
}

func TestMarkerAttributeSources(t *testing.T) {
	t.Setenv("GIT_ATTR_NOSYSTEM", "1")
	t.Chdir(t.TempDir())
	for _, tc := range []struct {
		name, index, worktree string
		absent, exempt        bool
	}{
		{"worktree removes exemption", "file conflict-markers=documented\n", "", false, false},
		{"worktree adds exemption", "", "file conflict-markers=documented\n", false, true},
		{"index fallback", "file conflict-markers=documented\n", "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GIT_ATTR_NOSYSTEM", "1")
			t.Chdir(t.TempDir())
			markerRepo(t)
			markerWrite(t, "file", "<<<<<<<\n")
			markerWrite(t, ".gitattributes", tc.index)
			markerGit(t, "add", "--", "file", ".gitattributes")
			markerWrite(t, ".gitattributes", tc.worktree)
			if tc.absent {
				require.NoError(t, os.Remove(".gitattributes"))
			}
			for _, staged := range []bool{false, true} {
				code, output := markerRun(t, staged, "file")
				if tc.exempt {
					require.Zero(t, code)
					require.Empty(t, output)
				} else {
					require.Equal(t, 1, code)
					require.Equal(t, "file  1 marker line(s)\n", output)
				}
			}
		})
	}
}

func TestMarkerUsage(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"markers", "scan"}, {"markers", "scan", "--staged"}} {
		var out, stderr bytes.Buffer
		require.Equal(t, 2, agentrepo.Run(args, nil, &out, &stderr))
		require.Empty(t, out.String())
		require.Equal(t, "usage: marker-scan.sh [--staged] <path> [path ...]\n", stderr.String())
	}
}
