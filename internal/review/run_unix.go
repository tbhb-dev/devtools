// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//go:build darwin || linux

package review

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"
)

//nolint:gochecknoinits // Select the process-group implementation before any review runs.
func init() {
	run = runUnix
}

// runUnix waits for both the reviewer and its output streams. It kills the group
// before returning and waits for the direct child; orphaned descendants are
// reaped by the operating system. Descendants must remain in the process group.
func runUnix(ctx context.Context, timeout time.Duration, args []string, in io.Reader, out, stderr io.Writer) int {
	if timeout <= 0 || len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "review: a positive timeout and a command are required")
		return Failed
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// The command is selected by machine configuration, not repository content.
	//nolint:gosec,noctx // Run the supplied command; cancellation also covers output EOF after the leader exits.
	cmd := exec.Command(args[0], args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdin = in
	// Hide *os.File so Wait also waits for EOF from descendants holding output.
	cmd.Stdout = struct{ io.Writer }{out}
	cmd.Stderr = struct{ io.Writer }{stderr}
	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintf(stderr, "review: start: %v\n", err)
		return Failed
	}
	defer killGroup(cmd.Process.Pid, stderr)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if ctx.Err() == nil {
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "review: command: %v\n", err)
				return Failed
			}
			return 0
		}
	case <-ctx.Done():
		killGroup(cmd.Process.Pid, stderr)
		<-done
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		_, _ = fmt.Fprintf(stderr, "review: timed out after %s\n", timeout)
		return Timeout
	}
	_, _ = fmt.Fprintln(stderr, "review: canceled")
	return Canceled
}

func killGroup(pid int, stderr io.Writer) {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		_, _ = fmt.Fprintf(stderr, "review: kill process group: %v\n", err)
	}
}
