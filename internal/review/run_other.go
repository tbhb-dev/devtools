// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Keep the fallback visible when Linux resolves merged cross-platform coverage.

package review

import (
	"context"
	"fmt"
	"io"
	"time"
)

// runUnsupported refuses platforms without the supported process-group lifecycle.
func runUnsupported(_ context.Context, _ time.Duration, _ []string, _ io.Reader, _, stderr io.Writer) int {
	_, _ = fmt.Fprintln(stderr, "review run requires macOS or Linux")
	return Failed
}
