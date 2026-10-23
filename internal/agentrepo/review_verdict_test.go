// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestReviewVerdict(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ reply, want string }{
		{"VERDICT: PASS\n", "pass\n"},
		{"VERDICT: CHANGES REQUIRED\nfinding\n", "changes\n"},
		{"commentary\nVERDICT: PASS\n", "unreadable\n"},
	} {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			file := filepath.Join(t.TempDir(), "reply")
			require.NoError(t, os.WriteFile(file, []byte(tt.reply), 0o600))
			var out, stderr bytes.Buffer
			code := agentrepo.Run([]string{"review", "verdict", "--file", file}, nil, &out, &stderr)
			require.Zero(t, code, stderr.String())
			require.Equal(t, tt.want, out.String())
			require.Empty(t, stderr.String())
		})
	}
}

func TestReviewVerdictInputFailure(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"review", "verdict"},
		{"review", "verdict", "--file", filepath.Join(t.TempDir(), "missing")},
		{"review", "verdict", "--file", t.TempDir()},
	} {
		var out, stderr bytes.Buffer
		require.NotZero(t, agentrepo.Run(args, nil, &out, &stderr))
		require.Empty(t, out.String())
		require.NotEmpty(t, stderr.String())
	}
}

func TestReviewVerdictOutputFailure(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "reply")
	require.NoError(t, os.WriteFile(file, []byte("VERDICT: PASS\n"), 0o600))
	var stderr bytes.Buffer
	want := io.ErrClosedPipe
	code := agentrepo.Run([]string{"review", "verdict", "--file", file}, nil, verdictErrorWriter{want}, &stderr)
	require.NotZero(t, code)
	require.Contains(t, stderr.String(), want.Error())
}

type verdictErrorWriter struct{ err error }

func (w verdictErrorWriter) Write(p []byte) (int, error) {
	return len(p) / 2, w.err
}
