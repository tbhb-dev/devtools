// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//go:build !darwin && !linux

package review_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/review"
)

func TestUnsupported(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	require.Equal(t, review.Failed, review.Run(t.Context(), time.Second, []string{"reviewer"}, nil, &out, &out))
	require.Contains(t, out.String(), "requires macOS or Linux")
}
