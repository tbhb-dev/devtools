// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func verdictFile(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reply")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReviewVerdictCommand(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ input, want string }{
		{"VERDICT: PASS\n", "pass\n"},
		{"VERDICT: CHANGES REQUIRED\nfindings", "changes\n"},
		{"", "unreadable\n"},
	} {
		var out, stderr bytes.Buffer
		code := agentrepo.Run([]string{"review", "verdict", "--file", verdictFile(t, tt.input)}, nil, &out, &stderr)
		if code != 0 || out.String() != tt.want {
			t.Fatalf("code %d output %q stderr %s", code, &out, &stderr)
		}
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
		if code := agentrepo.Run(args, nil, &out, &stderr); code == 0 || out.Len() != 0 {
			t.Fatalf("code %d output %q", code, &out)
		}
	}
}

type verdictWriter struct {
	n   int
	err error
}

func (w verdictWriter) Write(p []byte) (int, error) { return min(w.n, len(p)), w.err }

func TestReviewVerdictOutputFailure(t *testing.T) {
	t.Parallel()
	for _, out := range []io.Writer{verdictWriter{err: io.ErrClosedPipe}, verdictWriter{n: 2, err: io.ErrClosedPipe}, verdictWriter{n: 2}} {
		var stderr bytes.Buffer
		code := agentrepo.Run(
			[]string{"review", "verdict", "--file", verdictFile(t, "VERDICT: PASS\n")},
			nil,
			out,
			&stderr,
		)
		if code == 0 {
			t.Fatal("accepted a failed or partial write")
		}
	}
}
