// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package signing

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	env := map[string]string{}
	get := func(k string) string { return env[k] }
	c, err := ReadConfig(home, get)
	require.NoError(t, err)
	require.Equal(t, time.Hour, c.TTL)
	require.Equal(t, 45*time.Minute, c.Reload)
	path := filepath.Join(home, "signing.env")
	env["DEVTOOLS_SIGNING_CONFIG"] = path
	require.NoError(
		t,
		os.WriteFile(
			path,
			[]byte(
				"# settings\nDEVTOOLS_SIGNING_PROVIDER=file\nDEVTOOLS_SIGNING_KEY_TTL=600\nDEVTOOLS_SIGNING_RELOAD=300\n",
			),
			0o600,
		),
	)
	env["DEVTOOLS_SIGNING_KEY_TTL"] = "900"
	c, err = ReadConfig(home, get)
	require.NoError(t, err)
	require.Equal(t, "file", c.Provider)
	require.Equal(t, 15*time.Minute, c.TTL)
	for _, value := range []string{"bad", "0", "200"} {
		env["DEVTOOLS_SIGNING_KEY_TTL"] = value
		_, err = ReadConfig(home, get)
		require.Error(t, err)
	}
	delete(env, "DEVTOOLS_SIGNING_KEY_TTL")
	env["DEVTOOLS_SIGNING_RELOAD"] = "bad"
	_, err = ReadConfig(home, get)
	require.Error(t, err)
	delete(env, "DEVTOOLS_SIGNING_RELOAD")
	env["DEVTOOLS_SIGNING_PROVIDER"] = "unknown"
	_, err = ReadConfig(home, get)
	require.Error(t, err)
	delete(env, "DEVTOOLS_SIGNING_PROVIDER")
	require.NoError(t, os.WriteFile(path, []byte("echo unsafe"), 0o600))
	_, err = ReadConfig(home, get)
	require.Error(t, err)
	env["DEVTOOLS_SIGNING_CONFIG"] = home
	_, err = ReadConfig(home, get)
	require.Error(t, err)
}
