//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestGitHubSubprocess(t *testing.T) {
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(`#!/bin/sh
printf 'call\n' >>"$GH_CALLS"
printf '%s' "$GH_OUT"
printf '%s' "$GH_ERR" >&2
if [ "$GH_STATUS" = 143 ]; then kill -TERM "$$"; fi
exit "$GH_STATUS"
`), 0o600))
	//nolint:gosec // Fake gh must be executable to test the subprocess contract.
	require.NoError(t, os.Chmod(filepath.Join(bin, "gh"), 0o700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	calls := filepath.Join(bin, "calls")
	t.Setenv("GH_CALLS", calls)
	for _, tc := range []struct {
		name, stderr string
		status       int
		args         []string
		guidance     string
	}{
		{"signal", "terminated\n", 143, []string{"api", "user"}, ""},
		{"authentication", "gh: Bad credentials (HTTP 401)\n", 4, []string{"api", "user"}, ""},
		{"permission", "gh: Resource not accessible (HTTP 403)\n", 1, []string{"api", "user"}, ""},
		{"rest", "gh: API rate limit exceeded for user ID 1.\n", 1, []string{"api", "user"}, "primary REST"},
		{"already", "gh: API rate limit already exceeded for user ID 1.", 3, []string{"api", "user"}, "primary REST"},
		{"graphql prefix", "GraphQL: API rate limit exceeded for user ID 1.\n", 1, []string{"api", "user"}, "primary GraphQL"},
		{"graphql command", "gh: API rate limit exceeded for user ID 1.\n", 1, []string{"api", "graphql"}, "primary GraphQL"},
		{"subcommand", "gh: API rate limit exceeded for user ID 1.\n", 1, []string{"pr", "view", "7"}, "primary GraphQL"},
		{"secondary", "GraphQL: secondary rate limit\n", 1, []string{"api", "graphql"}, "secondary rate limit"},
		{"secondary rest", "gh: secondary rate limit\n", 1, []string{"api", "user"}, "secondary rate limit"},
		{"no prefix", "warning: API rate limit exceeded for user ID 1.\n", 3, []string{"api", "user"}, ""},
		{"no space", "gh:API rate limit exceeded for user ID 1.\n", 3, []string{"api", "user"}, ""},
		{"success", "gh: API rate limit exceeded for user ID 1.\n", 0, []string{"api", "user"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(calls, nil, 0o600))
			t.Setenv("GH_OUT", "gh: API rate limit exceeded for user ID 1.\npartial")
			t.Setenv("GH_ERR", tc.stderr)
			t.Setenv("GH_STATUS", strconv.Itoa(tc.status))
			var out, stderr bytes.Buffer
			code := agentrepo.Run(append([]string{"github", "run"}, tc.args...), strings.NewReader(""), &out, &stderr)
			require.Equal(t, os.Getenv("GH_OUT"), out.String())
			if tc.guidance == "" {
				require.Equal(t, tc.status, code)
				require.Equal(t, tc.stderr, stderr.String())
			} else {
				require.Equal(t, 76, code)
				require.Contains(t, stderr.String(), tc.guidance)
				require.True(t, strings.HasSuffix(stderr.String(), "No automatic retry was made.\n"+tc.stderr))
			}
			data, err := os.ReadFile(calls)
			require.NoError(t, err)
			require.Equal(t, "call\n", string(data))
		})
	}
}
