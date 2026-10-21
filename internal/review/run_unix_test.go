// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//go:build darwin || linux

package review_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/review"
)

func TestRun(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, script string
		want         int
	}{
		{"hang", `echo $$ > "$1"; exec sleep 60`, review.Timeout},
		{"hanging child", `sh -c 'echo $$ > "$1"; exec sleep 60' _ "$1" & wait`, review.Timeout},
		{"grandchild holds stdout", `sh -c 'echo $$ > "$1"; exec sleep 60' _ "$1" & exit 0`, review.Timeout},
		{"pass then hang", `echo $$ > "$1"; echo 'VERDICT: PASS'; exec sleep 60`, review.Timeout},
		{"nonzero", `echo 'VERDICT: PASS'; exit 7`, review.Failed},
		{"reserved exit", `exit 124`, review.Failed},
		{"normal", `cat`, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pidFile := filepath.Join(t.TempDir(), "pid")
			var out, stderr bytes.Buffer
			started := time.Now()
			got := review.Run(t.Context(), time.Second, []string{"sh", "-c", tt.script, "_", pidFile},
				strings.NewReader("VERDICT: PASS\n"), &out, &stderr)
			require.Equal(t, tt.want, got, stderr.String())
			require.Less(t, time.Since(started), 5*time.Second)
			if tt.want == review.Timeout {
				require.Contains(t, stderr.String(), "timed out")
				assertGone(t, waitForPID(t, pidFile))
			}
			if tt.name == "normal" {
				require.Equal(t, "VERDICT: PASS\n", out.String())
			}
		})
	}
}

func waitForPID(t *testing.T, pidFile string) int {
	t.Helper()
	var pid int
	require.Eventually(
		t,
		func() bool {
			data, err := os.ReadFile(pidFile)
			if err != nil {
				return false
			}
			pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
			return err == nil
		},
		3*time.Second,
		10*time.Millisecond,
	)
	return pid
}

func assertGone(t *testing.T, pid int) {
	t.Helper()
	require.Eventually(
		t,
		func() bool { return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) },
		3*time.Second,
		10*time.Millisecond,
	)
}

func TestCancel(t *testing.T) {
	t.Parallel()
	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var out, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- review.Run(ctx, time.Minute,
			[]string{"sh", "-c", `sh -c 'echo $$ > "$1"; exec sleep 60' _ "$1" & wait`, "_", pidFile},
			nil, &out, &stderr)
	}()
	pid := waitForPID(t, pidFile)
	cancel()
	select {
	case got := <-done:
		require.Equal(t, review.Canceled, got, stderr.String())
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not finish")
	}
	assertGone(t, pid)
}

func TestInvalid(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		timeout time.Duration
		args    []string
	}{
		{0, []string{"sh"}}, {time.Second, nil}, {time.Second, []string{"/nonexistent/reviewer"}},
	} {
		var out bytes.Buffer
		require.Equal(t, review.Failed, review.Run(t.Context(), tt.timeout, tt.args, nil, &out, &out))
	}
}

func TestFileOutput(t *testing.T) {
	t.Parallel()
	pidFile := filepath.Join(t.TempDir(), "pid")
	out, err := os.CreateTemp(t.TempDir(), "output")
	require.NoError(t, err)
	defer out.Close()
	var stderr bytes.Buffer
	got := review.Run(t.Context(), time.Second,
		[]string{"sh", "-c", `sh -c 'echo $$ > "$1"; exec sleep 60' _ "$1" & exit 0`, "_", pidFile}, nil, out, &stderr)
	require.Equal(t, review.Timeout, got, stderr.String())
	assertGone(t, waitForPID(t, pidFile))
}
