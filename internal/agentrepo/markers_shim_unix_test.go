//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func markerShim(t *testing.T, shim string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "bash")
	cmd.Args = append(cmd.Args, shim)
	cmd.Args = append(cmd.Args, args...)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err := cmd.Run()
	if err == nil {
		return 0, out.String(), stderr.String()
	}
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	return exit.ExitCode(), out.String(), stderr.String()
}

func TestMarkerEntryPoint(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	shim := filepath.Join(root, "packages/agents/source/skills/rebase/shared/scripts/marker-scan.sh")
	bin := t.TempDir()
	build := exec.CommandContext(t.Context(), "go", "build", "-o")
	build.Args = append(build.Args, filepath.Join(bin, "agentrepo"), "./cmd/agentrepo")
	build.Dir = root
	output, err := build.CombinedOutput()
	require.NoError(t, err, "%s", output)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Chdir(t.TempDir())
	markerRepo(t)
	markerWrite(t, "one", "<<<<<<< left\n=======\n>>>>>>> right\n")
	markerWrite(t, "two", "|||||||\n")
	markerWrite(t, "clean", "clean\n")
	markerGit(t, "add", "--", "one", "two", "clean")
	// Recorded from the previous shell scanner on these same fixtures.
	for _, tc := range []struct {
		name        string
		args        []string
		code        int
		out, stderr string
	}{
		{"clean", []string{"clean"}, 0, "", ""},
		{"one", []string{"one"}, 1, "one  2 marker line(s)\n", ""},
		{"several", []string{"two", "one"}, 1, "two  1 marker line(s)\none  2 marker line(s)\n", ""},
		{"repeated", []string{"one", "one"}, 1, "one  2 marker line(s)\none  2 marker line(s)\n", ""},
		{"empty", []string{""}, 0, "", ""},
		{"mixed empty", []string{"", "one", ""}, 1, "one  2 marker line(s)\n", ""},
		{"usage", nil, 2, "", "usage: marker-scan.sh [--staged] <path> [path ...]\n"},
		{"staged usage", []string{"--staged"}, 2, "", "usage: marker-scan.sh [--staged] <path> [path ...]\n"},
	} {
		code, stdout, stderr := markerShim(t, shim, tc.args...)
		require.Equal(t, tc.code, code)
		require.Equal(t, tc.out, stdout)
		require.Equal(t, tc.stderr, stderr)
	}
	markerTiming(t, shim)
}

func markerTiming(t *testing.T, shim string) {
	t.Helper()
	paths := make([]string, 10_000)
	var expected strings.Builder
	for i := range paths {
		paths[i] = fmt.Sprintf("files/%05d", i)
		content := strings.Repeat("ordinary line\n", 50)
		if i%2500 == 0 {
			content = "<<<<<<< left\n" + strings.Repeat("ordinary line\n", 49)
			fmt.Fprintf(&expected, "%s  1 marker line(s)\n", paths[i])
		}
		markerWrite(t, paths[i], content)
	}
	markerGit(t, "add", "--", "files")
	start := time.Now()
	code, stdout, stderr := markerShim(t, shim, paths...)
	elapsed := time.Since(start)
	t.Logf("entry point scanned tracked fixture in %s", elapsed)
	require.Less(t, elapsed, 2*time.Second)
	require.Equal(t, 1, code)
	require.Equal(t, expected.String(), stdout)
	require.Empty(t, stderr)
}
