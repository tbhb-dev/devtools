// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package review

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnsupportedPlatformFailsClosed(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	require.Equal(t, Failed, runUnsupported(t.Context(), 0, nil, nil, nil, &stderr))
	require.Equal(t, "review run requires macOS or Linux\n", stderr.String())
}
