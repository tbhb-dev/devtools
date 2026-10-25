//go:build unix

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package versions_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnreadableSite(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	file := filepath.Join(root, "README.md")
	require.NoError(t, os.Chmod(file, 0))
	t.Cleanup(func() { require.NoError(t, os.Chmod(file, 0o600)) })
	require.Contains(
		t,
		check(t, root, "1.2.3", 1),
		"README.md:1 [error] version-literal-stale  cannot read file",
		"AC8: unreadable file",
	)
}
