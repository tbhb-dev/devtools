// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLintSourceModes(t *testing.T) {
	for _, tc := range []struct {
		mode, pattern string
		gate          []string
	}{
		{"shell", "*.sh", []string{"shellcheck", "-o", "check-extra-masked-returns", "--fix"}},
		{"curl", "*.sh", []string{"uv", "run", "--script", "payload/scripts/check_curl_fail.py", "--fix"}},
		{"shell-fmt", "*.sh", []string{"shfmt", "-d", "--fix"}},
		{"prose", "--fix", []string{"vale", "--output=ai-tells-agent.tmpl", "--glob=!vendor/*"}},
		{"markdown-wrap", "*.md", []string{"guard-markdown", "--fix"}},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Setenv("DEVTOOLS_DIR", "payload")
			t.Setenv("DEVTOOLS_GENERATED_PATHS", "generated output/\n\ncustom/file.sh\n")
			t.Setenv("DEVTOOLS_PROSE_GLOB", "!vendor/*")
			var calls [][]string
			execute := func(name string, args []string, out io.Writer) int {
				calls = append(calls, append([]string{name}, args...))
				if name == "git" {
					_, err := io.WriteString(out, "untracked space.sh\x00tracked\nnewline.sh\x00")
					require.NoError(t, err)
				}
				return 0
			}
			require.Zero(t, lintSource(t.Context(), tc.mode, []string{"--fix"}, io.Discard, io.Discard, execute))
			wantGit := []string{
				"git",
				"ls-files",
				"-z",
				"--cached",
				"--others",
				"--exclude-standard",
				"--",
				tc.pattern,
				":(exclude)vendor/**",
				":(exclude)apm_modules/**",
				":(exclude)generated output",
				":(exclude)generated output/**",
				":(exclude)custom/file.sh",
				":(exclude)custom/file.sh/**",
			}
			require.Equal(t, wantGit, calls[0])
			require.Equal(t, []string{"uv", "run", "--script", "payload/scripts/filter-apm-deployed.py"}, calls[1][:4])
			require.Equal(t, append(tc.gate, "untracked space.sh", "tracked\nnewline.sh"), calls[2])
		})
	}
}

func TestLintSourceStopsBeforeGate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                                 string
		gitStatus, filterStatus, want, calls int
		data                                 string
	}{
		{"empty", 0, 0, 0, 2, ""},
		{"git", 23, 0, 23, 1, "partial.sh\x00"},
		{"filter", 0, 24, 24, 2, "partial.sh\x00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			execute := func(name string, _ []string, out io.Writer) int {
				calls++
				if name == "git" {
					_, err := io.WriteString(out, tc.data)
					require.NoError(t, err)
					return tc.gitStatus
				}
				return tc.filterStatus
			}
			require.Equal(t, tc.want, lintSource(t.Context(), "shell", nil, io.Discard, io.Discard, execute))
			require.Equal(t, tc.calls, calls)
		})
	}
}

func TestLintSourceDefaultsAndProsePatterns(t *testing.T) {
	t.Setenv("DEVTOOLS_GENERATED_PATHS", "")
	t.Setenv("DEVTOOLS_PROSE_GLOB", "")
	for _, patterns := range [][]string{nil, {"docs/*", "README.md"}} {
		var calls [][]string
		execute := func(name string, argv []string, out io.Writer) int {
			calls = append(calls, append([]string{name}, argv...))
			if name == "git" {
				_, err := io.WriteString(out, "owned.md\x00")
				require.NoError(t, err)
			}
			return 0
		}
		require.Zero(t, lintSource(t.Context(), "prose", patterns, io.Discard, io.Discard, execute))
		if len(patterns) == 0 {
			patterns = []string{"*"}
		}
		want := append(
			[]string{"git", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--"},
			patterns...,
		)
		want = append(want, ":(exclude)vendor/**", ":(exclude)apm_modules/**")
		require.Equal(t, want, calls[0])
		require.Equal(t, []string{"vale", "--output=ai-tells-agent.tmpl", "--glob=*", "owned.md"}, calls[2])
	}
}

func TestLintSourceGateResult(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	execute := func(name string, _ []string, out io.Writer) int {
		switch name {
		case "git":
			_, err := io.WriteString(out, "owned.sh\x00")
			require.NoError(t, err)
		case "shellcheck":
			_, err := io.WriteString(out, "gate output\n")
			require.NoError(t, err)
			_, err = io.WriteString(&stderr, "gate diagnostic\n")
			require.NoError(t, err)
			return 19
		}
		return 0
	}
	require.Equal(t, 19, lintSource(context.Background(), "shell", nil, &stdout, &stderr, execute))
	require.Equal(t, "gate output\n", stdout.String())
	require.Equal(t, "gate diagnostic\n", stderr.String())
}

func TestLintExecutionStreamsAndStatus(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	require.Zero(t, executeLint(t.Context(), "git", []string{"--version"}, &stdout, &stderr))
	require.Contains(t, stdout.String(), "git version")
	require.Empty(t, stderr.String())
	stdout.Reset()
	require.NotZero(t, executeLint(t.Context(), "git", []string{"--invalid-devtools-option"}, &stdout, &stderr))
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "unknown option")
}
