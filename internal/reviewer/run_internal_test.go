// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package reviewer

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/ghapp"
)

func TestRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell executable fixtures")
	}
	// Environment and PATH changes are process-global.
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	require.NoError(t, os.Mkdir(bin, 0o700))
	//nolint:gosec // This fixture is an executable owned by the test.
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nexit 99\n"), 0o700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_TOKEN", "old-token")
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "provider-secret")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	keyFile := filepath.Join(home, "key.pem")
	require.NoError(
		t,
		os.WriteFile(
			keyFile,
			pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}),
			0o600,
		),
	)
	now := time.Now().UTC().Truncate(time.Second)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, "/app/installations/42/access_tokens", r.URL.Path)
		_, writeErr := fmt.Fprintf(
			w,
			`{"token":"installation-token","expires_at":%q}`,
			now.Add(time.Hour).Format(time.RFC3339),
		)
		assert.NoError(t, writeErr)
	}))
	defer server.Close()
	args := []string{"pr", "review", "123", "--approve", "--body", "literal $value and spaces", "-R", "owner/repo"}
	executed := 0
	runner := Runner{
		Config: Config{
			Provider:     "file",
			AppID:        "123",
			KeySource:    keyFile,
			Installation: "42",
			CacheDir:     filepath.Join(home, "cache"),
		},
		Client: ghapp.Client{HTTP: server.Client(), BaseURL: server.URL},
		Now:    func() time.Time { return now },
		Exec: func(binary string, argv, env []string) error {
			executed++
			require.Equal(t, filepath.Join(bin, "gh"), binary)
			require.Equal(t, append([]string{"gh"}, args...), argv)
			require.Contains(t, env, "GH_TOKEN=installation-token")
			require.NotContains(t, env, "GH_TOKEN=old-token")
			require.NotContains(t, env, "OP_SERVICE_ACCOUNT_TOKEN=provider-secret")
			return nil
		},
	}
	require.NoError(t, runner.Run(t.Context(), args))
	require.NoError(t, os.Remove(keyFile))
	require.NoError(t, runner.Run(t.Context(), args))
	require.Equal(t, 1, calls)
	require.Equal(t, 2, executed)
	now = now.Add(55 * time.Minute)
	require.Error(t, runner.Run(t.Context(), args))
	require.Equal(t, 2, executed)
}

func TestConfig(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	path := filepath.Join(home, "reviewer.env")
	keyFile := filepath.Join(home, "key.pem")
	env := map[string]string{"DEVTOOLS_REVIEWER_CONFIG": path}
	get := func(key string) string { return env[key] }
	require.NoError(
		t,
		os.WriteFile(
			path,
			[]byte(
				"# literal values\nDEVTOOLS_REVIEWER_PROVIDER=file\nDEVTOOLS_REVIEWER_APP_ID=123\nDEVTOOLS_REVIEWER_KEY_FILE="+keyFile+"\n",
			),
			0o600,
		),
	)
	c, err := ReadConfig(home, get)
	require.NoError(t, err)
	require.Equal(t, "123", c.AppID)
	require.Equal(t, keyFile, c.KeySource)
	env["DEVTOOLS_REVIEWER_APP_ID"] = "456"
	c, err = ReadConfig(home, get)
	require.NoError(t, err)
	require.Equal(t, "456", c.AppID)
	for _, id := range []string{"invalid", "0", "-1"} {
		env["DEVTOOLS_REVIEWER_INSTALLATION_ID"] = id
		_, err = ReadConfig(home, get)
		require.Error(t, err)
	}
	delete(env, "DEVTOOLS_REVIEWER_INSTALLATION_ID")
	env["DEVTOOLS_REVIEWER_PROVIDER"] = "invalid"
	_, err = ReadConfig(home, get)
	require.Error(t, err)
	env["DEVTOOLS_REVIEWER_PROVIDER"] = "op"
	_, err = ReadConfig(home, get)
	require.Error(t, err)
	env["DEVTOOLS_REVIEWER_APP_ID_REFERENCE"] = "app-id-reference"
	env["DEVTOOLS_REVIEWER_OP_REFERENCE"] = "key-reference"
	c, err = ReadConfig(home, get)
	require.NoError(t, err)
	require.Equal(t, env["DEVTOOLS_REVIEWER_OP_REFERENCE"], c.KeySource)
	require.NoError(t, os.WriteFile(path, []byte("export DEVTOOLS_REVIEWER_APP_ID=bad"), 0o600))
	_, err = ReadConfig(home, get)
	require.Error(t, err)
}
