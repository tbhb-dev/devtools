//go:build !windows

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

func TestSizePublishAuthor(t *testing.T) {
	t.Chdir(t.TempDir())
	git := func(args ...string) {
		t.Helper()
		out, err := exec.CommandContext(t.Context(), "git", args...).CombinedOutput()
		require.NoError(t, err, "%s", out)
	}
	git("init", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	git("config", "commit.gpgsign", "false")
	git("config", "core.hooksPath", "/dev/null")
	git("commit", "--allow-empty", "-m", "base")
	git("checkout", "-b", "topic")
	require.NoError(t, os.WriteFile("a.md", []byte(strings.Repeat("prose\n", 2001)), 0o600))
	git("add", "a.md")
	git("commit", "-m", "chore: large prose")
	bin := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(bin, "gh"),
			[]byte("#!/bin/sh\nprintf '%s' \"$AUTHOR_JSON\"\nexit \"${AUTHOR_STATUS:-0}\"\n"),
			0o600,
		),
	)
	info, err := os.Stat(filepath.Join(bin, "gh"))
	require.NoError(t, err)
	const ownerExecute = 0o100
	require.NoError(t, os.Chmod(filepath.Join(bin, "gh"), info.Mode()|ownerExecute))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tt := range []struct {
		author string
		want   int
	}{
		{`[{"author":{"login":"app/renovate","is_bot":true}}]`, 0},
		{`[{"author":{"login":"renovate[bot]","is_bot":true}}]`, 0},
		{`[{"author":{"login":"app/renovate","is_bot":false}}]`, 1},
		{`[{"author":{"login":"dependabot[bot]","is_bot":true}}]`, 1},
		{`[]`, 1},
		{`invalid`, 2},
	} {
		t.Setenv("AUTHOR_JSON", tt.author)
		var out bytes.Buffer
		require.Equal(
			t,
			tt.want,
			agentrepo.Run([]string{"size", "check", "--publish", "--base", "main"}, nil, &out, &out),
			"%s",
			&out,
		)
	}
	t.Setenv("AUTHOR_STATUS", "1")
	var out bytes.Buffer
	require.Equal(t, 2, agentrepo.Run([]string{"size", "check", "--publish", "--base", "main"}, nil, &out, &out))
	require.Contains(t, out.String(), "read pull request author")
}
