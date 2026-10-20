// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package credentials

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadSettings(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "settings.env")
	values := map[string]string{"PROVIDER": "op"}
	require.NoError(t, ReadSettings(path, "TEST_", values))
	require.NoError(
		t,
		os.WriteFile(path, []byte("# comment\n\nTEST_PROVIDER=file\nTEST_REFERENCE=literal $value\n"), 0o600),
	)
	require.NoError(t, ReadSettings(path, "TEST_", values))
	require.Equal(t, map[string]string{"PROVIDER": "file", "REFERENCE": "literal $value"}, values)
	for _, data := range []string{"export TEST_PROVIDER=file", "OTHER_PROVIDER=file", strings.Repeat("x", 70000)} {
		require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
		require.Error(t, ReadSettings(path, "TEST_", values))
	}
	require.Error(t, ReadSettings(filepath.Dir(path), "TEST_", values))
}
