//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestPostRequirement(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, args := range [][]string{{"post", "require"}, {"post", "run", "pr", "comment", "1"}} {
		var out, stderr bytes.Buffer
		code := agentrepo.Run(args, nil, &out, &stderr)
		require.Equal(t, 2, code)
		require.Empty(t, out.String())
		require.Equal(t, "gh-post is missing; install the gh-post tool before publishing content.\n", stderr.String())
	}
}

func TestPostForwarding(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	path := filepath.Join(bin, "gh-post")
	//nolint:gosec // This fixture in a private temporary directory must execute as gh-post.
	require.NoError(t, os.WriteFile(path, []byte(`#!/bin/sh
printf "attempt\n" >>"$POST_ATTEMPTS"
for arg do printf '%s\000' "$arg"; done
/bin/cat
printf 'diagnostic\n' >&2
[ "$POST_STATUS" != TERM ] || kill -TERM "$$"
exit "$POST_STATUS"
`), 0o700))
	args := []string{
		"pr", "", "two words", "tab\there", "line\nbreak", "'\"$();*?\\",
		"--help", "--", "--body-file", "body",
	}
	for _, status := range []struct {
		value string
		code  int
	}{{"0", 0}, {"17", 17}, {"TERM", 143}} {
		t.Setenv("POST_STATUS", status.value)
		attempts := filepath.Join(t.TempDir(), "attempts")
		t.Setenv("POST_ATTEMPTS", attempts)
		var out, stderr bytes.Buffer
		input := "stdin\t ' $ * \\\nsecond line\x00tail"
		code := agentrepo.Run(append([]string{"post", "run"}, args...), strings.NewReader(input), &out, &stderr)
		require.Equal(t, status.code, code)
		require.Equal(t, strings.Join(args, "\x00")+"\x00"+input, out.String())
		require.Equal(t, "diagnostic\n", stderr.String())
		writes, err := os.ReadFile(attempts)
		require.NoError(t, err)
		require.Equal(t, "attempt\n", string(writes))
	}
	var out, stderr bytes.Buffer
	require.Zero(t, agentrepo.Run([]string{"post", "require"}, nil, &out, &stderr))
	require.Empty(t, out.String())
	require.Empty(t, stderr.String())
	// Help is a capability probe and must never execute gh-post.
	require.Zero(t, agentrepo.Run([]string{"--help"}, nil, &out, &stderr))
	require.Contains(t, out.String(), "post")
	require.Empty(t, stderr.String())
}
