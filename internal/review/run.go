// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package review bounds a reviewer and its process group.
package review

import (
	"context"
	"io"
	"time"
)

//nolint:gochecknoglobals // Platform initialization selects the runner once before use.
var run = runUnsupported

// Run bounds a reviewer where process-group cleanup is supported.
func Run(ctx context.Context, timeout time.Duration, args []string, in io.Reader, out, stderr io.Writer) int {
	return run(ctx, timeout, args, in, out, stderr)
}

const (
	// Timeout is distinct from the review workflow's verdict statuses.
	Timeout = 124
	// Canceled reports interruption without accepting a verdict.
	Canceled = 130
	// Failed covers launch, I/O, and reviewer exit failures.
	Failed = 1
)
