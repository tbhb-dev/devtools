// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//go:build darwin || linux

package agentrepo_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
	"github.com/tbhb-dev/devtools/internal/review"
)

func TestReview(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		args []string
		want int
	}{
		{"normal", []string{"--timeout", "1s", "--", "sh", "-c", "cat"}, 0},
		{"timeout", []string{"--timeout", "50ms", "--", "sleep", "60"}, review.Timeout},
		{"no timeout", []string{"--", "true"}, review.Failed},
		{"no command", []string{"--timeout", "1s"}, 5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, stderr bytes.Buffer
			got := agentrepo.Run(
				append([]string{"review", "run"}, tt.args...),
				strings.NewReader("prompt"),
				&out,
				&stderr,
			)
			require.Equal(t, tt.want, got, stderr.String())
			if tt.want == 0 {
				require.Equal(t, "prompt", out.String())
			}
		})
	}
}

func TestReviewSignal(t *testing.T) {
	t.Parallel()
	// The helper is a separate process so signal handling never affects go test.
	if os.Getenv("DEVTOOLS_SIGNAL_HELPER") == "1" {
		os.Exit(agentrepo.Run([]string{
			"review", "run", "--timeout", "1m", "--", "sh", "-c",
			`echo $$ > "$1"; exec sleep 60`, "_", os.Getenv("DEVTOOLS_PID_FILE"),
		}, os.Stdin, os.Stdout, os.Stderr))
	}
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			t.Parallel()
			pidFile := filepath.Join(t.TempDir(), "pid")
			//nolint:gosec // Re-execute this test binary as an isolated signal target.
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestReviewSignal$")
			cmd.Env = append(os.Environ(), "DEVTOOLS_SIGNAL_HELPER=1", "DEVTOOLS_PID_FILE="+pidFile)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			require.NoError(t, cmd.Start())
			t.Cleanup(func() {
				if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
					t.Error(err)
				}
			})
			require.Eventually(t, func() bool {
				data, err := os.ReadFile(pidFile)
				return err == nil && strings.HasSuffix(string(data), "\n")
			}, 5*time.Second, 10*time.Millisecond)
			require.NoError(t, cmd.Process.Signal(sig))
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				var exit *exec.ExitError
				require.ErrorAs(t, err, &exit)
				require.Equal(t, review.Canceled, exit.ExitCode(), out.String())
			case <-time.After(5 * time.Second):
				t.Fatal("signal did not stop the runner")
			}
			data, err := os.ReadFile(pidFile)
			require.NoError(t, err)
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			require.NoError(t, err)
			require.ErrorIs(t, syscall.Kill(pid, 0), syscall.ESRCH)
		})
	}
}
