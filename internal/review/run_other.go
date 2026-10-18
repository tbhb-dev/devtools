// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//go:build !darwin && !linux

package review

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Run refuses platforms without the supported process-group lifecycle.
func Run(_ context.Context, _ time.Duration, _ []string, _ io.Reader, _, stderr io.Writer) int {
	_, _ = fmt.Fprintln(stderr, "review run requires macOS or Linux")
	return Failed
}
