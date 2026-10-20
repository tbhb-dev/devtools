// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//go:build !windows

package reviewer

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecUnixFailure(t *testing.T) {
	t.Parallel()
	require.Error(t, execUnix(filepath.Join(t.TempDir(), "missing"), []string{"gh"}, nil))
}
