// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package retro_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func runPreflight(t *testing.T, skill string, args ...string) (int, string, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	code := agentrepo.Run(append([]string{"retro", "preflight", "--skill-dir", skill}, args...), nil, &out, &stderr)
	return code, out.String(), stderr.String()
}

// Covers Bats bare-ID resolution and compares all seven sections byte for byte.
func TestRetroPreflightShellParity(t *testing.T) {
	skill, c, x := fixture(t)
	t.Setenv("DEVTOOLS_HARNESS", "claude")
	for _, transcript := range []string{c, x} {
		code, _, stderr := run(t, skill, transcript)
		require.Zero(t, code, stderr)
	}
	for _, arg := range []string{c, "session-claude-1", "cafe", x} {
		for _, loops := range []string{"", "2"} {
			t.Run(filepath.Base(arg)+"/loops="+loops, func(t *testing.T) {
				t.Setenv("ANALYZE_PREFLIGHT_LOOPS", loops)
				code, out, stderr := runPreflight(t, skill, arg)
				require.Zero(t, code, stderr)
				require.Empty(t, stderr)
				command := []string{filepath.Join(skill, "scripts/preflight.sh"), arg}
				script := exec.CommandContext(t.Context(), "bash", command...)
				shell, err := script.CombinedOutput()
				require.NoError(t, err, "%s", shell)
				require.Equal(t, transcriptLine(t, string(shell)), transcriptLine(t, out))
				require.Equal(t, withoutTranscript(string(shell)), withoutTranscript(out))
			})
		}
	}
}

// The shell prints the transcript in the spelling Git Bash uses on Windows. This test
// compares that line as one real location and every other byte exactly.
func transcriptLine(t *testing.T, output string) string {
	t.Helper()
	_, rest, _ := strings.Cut(output, "== transcript ==\n")
	line, _, _ := strings.Cut(rest, "\n")
	if runtime.GOOS != "windows" {
		return line
	}
	native, err := exec.CommandContext(t.Context(), "cygpath", "-w", line).Output()
	if err == nil {
		line = strings.TrimSpace(string(native))
	}
	resolved, err := filepath.EvalSymlinks(line)
	require.NoError(t, err)
	return filepath.ToSlash(resolved)
}

func withoutTranscript(output string) string {
	head, rest, _ := strings.Cut(output, "== transcript ==\n")
	_, rest, _ = strings.Cut(rest, "\n")
	return head + rest
}

// Covers Bats unresolved guidance.
func TestRetroPreflightRefusals(t *testing.T) {
	skill, c, _ := fixture(t)
	t.Setenv("DEVTOOLS_HARNESS", "claude")
	write(t, "not-json", "plain text")
	write(t, filepath.Join(os.Getenv("HOME"), ".claude/projects/other/session-claude-1.jsonl"), read(t, c))
	for _, tc := range []struct {
		args    []string
		message string
	}{
		{nil, "no path given"},
		{[]string{"absent"}, "not a readable file"},
		{[]string{"session-claude-1"}, "matches more than one"},
		{[]string{"not-json"}, "not JSON lines"},
	} {
		code, out, _ := runPreflight(t, skill, tc.args...)
		require.Zero(t, code)
		require.Contains(t, out, tc.message)
		require.Contains(t, out, "~/.claude/projects")
		require.Contains(t, out, "~/.codex/sessions")
	}
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Chmod(c, 0))
		code, out, _ := runPreflight(t, skill, c)
		require.Zero(t, code)
		require.Contains(t, out, "cannot read:")
		require.NoError(t, os.Chmod(c, 0o600))
	}
}

func TestRetroMissingUV(t *testing.T) {
	skill, c, _ := fixture(t)
	t.Setenv("PATH", t.TempDir())
	code, out, _ := runPreflight(t, skill, c)
	require.Zero(t, code)
	require.Contains(t, out, "MISSING")
}

func TestRetroPreflightFailuresAndFormat(t *testing.T) {
	_, c, _ := fixture(t)
	skill := stub(t)
	code, out, stderr := runPreflight(t, skill, c)
	require.Zero(t, code, stderr)
	require.Contains(t, out, strings.Join(numberLines(12), "\n")+"\n\n== lint ==")
	t.Setenv("ANALYZE_PREFLIGHT_LOOPS", "3")
	code, out, stderr = runPreflight(t, skill, c)
	require.Zero(t, code, stderr)
	require.Contains(t, out, "== index ==\n"+strings.Join(numberLines(24), "\n")+"\n\n== loops ==")
	require.Contains(t, out, "0\n1\n2\n\n== lint ==\n"+strings.Join(numberLines(14), "\n")+"\n\n== artifacts ==")
	for _, report := range []string{"meta", "index", "errors", "loops", "lint"} {
		t.Setenv("FAIL_REPORT", report)
		code, out, stderr = runPreflight(t, skill, c)
		require.Zero(t, code)
		require.Empty(t, stderr)
		require.Contains(t, out, "scaffolding failed — tell the operator")
		require.NotContains(t, out, "partial output")
	}
}

func numberLines(n int) []string {
	lines := make([]string, n)
	for i := range n {
		lines[i] = strconv.Itoa(i)
	}
	return lines
}
