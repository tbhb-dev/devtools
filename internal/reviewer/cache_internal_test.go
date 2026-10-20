// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package reviewer

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/ghapp"
)

func TestCache(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission semantics")
	}
	now := time.Now()
	c := Config{CacheDir: filepath.Join(t.TempDir(), "reviewer"), AppID: "123"}
	token := ghapp.Token{Value: "test-token", Expires: now.Add(time.Hour)}
	_, hit := cachedToken(c, "owner/repo", now)
	require.False(t, hit)
	require.NoError(t, storeToken(c, "owner/repo", token))
	got, hit := cachedToken(c, "owner/repo", now)
	require.True(t, hit)
	require.Equal(t, token.Value, got.Value)
	for _, stamp := range []time.Time{token.Expires.Add(-5 * time.Minute), token.Expires, token.Expires.Add(time.Hour)} {
		_, hit = cachedToken(c, "owner/repo", stamp)
		require.False(t, hit)
	}
	_, hit = cachedToken(c, "other/repo", now)
	require.False(t, hit)
	other := c
	other.AppID = "456"
	_, hit = cachedToken(other, "owner/repo", now)
	require.False(t, hit)
	path := cachePath(c, "owner/repo")
	//nolint:gosec // Exercise rejection of an insecure cache fixture.
	require.NoError(t, os.Chmod(path, 0o644))
	_, hit = cachedToken(c, "owner/repo", now)
	require.False(t, hit)
	require.NoError(t, storeToken(c, "owner/repo", token))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	//nolint:gosec // Exercise rejection of an insecure cache fixture.
	require.NoError(t, os.Chmod(c.CacheDir, 0o755))
	_, hit = cachedToken(c, "owner/repo", now)
	require.False(t, hit)
	require.NoError(t, storeToken(c, "owner/repo", token))
	info, err = os.Stat(c.CacheDir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	require.NoError(t, os.WriteFile(path, []byte("invalid"), 0o600))
	_, hit = cachedToken(c, "owner/repo", now)
	require.False(t, hit)
	require.NoError(t, os.Remove(path))
	secret := filepath.Join(t.TempDir(), "unrelated")
	require.NoError(t, os.WriteFile(secret, []byte("unchanged"), 0o600))
	require.NoError(t, os.Symlink(secret, path))
	_, hit = cachedToken(c, "owner/repo", now)
	require.False(t, hit)
	require.NoError(t, storeToken(c, "owner/repo", token))
	data, err := os.ReadFile(secret)
	require.NoError(t, err)
	require.Equal(t, "unchanged", string(data))
}
