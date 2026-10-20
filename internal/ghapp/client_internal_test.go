// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMint(t *testing.T) {
	t.Parallel()
	for _, configured := range []string{"", "42"} {
		t.Run("installation="+configured, func(t *testing.T) {
			t.Parallel()
			lookups, mints := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Bearer test-jwt", r.Header.Get("Authorization"))
				assert.Equal(t, "2022-11-28", r.Header.Get("X-Github-Api-Version"))
				if r.Method == http.MethodGet {
					lookups++
					assert.Equal(t, "/repos/owner/repo/installation", r.URL.Path)
					_, err := fmt.Fprint(w, `{"id":42}`)
					assert.NoError(t, err)
					return
				}
				mints++
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/app/installations/42/access_tokens", r.URL.Path)
				var body map[string]any
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(
					t,
					map[string]any{
						"repositories": []any{"repo"},
						"permissions":  map[string]any{"pull_requests": "write", "metadata": "read"},
					},
					body,
				)
				_, err := fmt.Fprint(w, `{"token":"test-installation-token","expires_at":"2030-01-01T00:00:00Z"}`)
				assert.NoError(t, err)
			}))
			defer server.Close()
			client := Client{HTTP: server.Client(), BaseURL: server.URL}
			token, err := client.Mint(t.Context(), "test-jwt", "owner", "repo", configured)
			require.NoError(t, err)
			require.Equal(t, "test-installation-token", token.Value)
			require.Equal(t, 2030, token.Expires.Year())
			require.Equal(t, 1, mints)
			if configured == "" {
				require.Equal(t, 1, lookups)
			} else {
				require.Zero(t, lookups)
			}
		})
	}
}

func TestMintErrors(t *testing.T) {
	t.Parallel()
	for _, response := range []struct {
		status int
		body   string
	}{{403, "sensitive-body"}, {200, "bad"}, {200, `{}`}, {200, `{"token":"x"}`}} {
		t.Run(response.body, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(response.status)
				_, err := fmt.Fprint(w, response.body)
				assert.NoError(t, err)
			}))
			defer server.Close()
			client := Client{HTTP: server.Client(), BaseURL: server.URL}
			for _, id := range []string{"", "42"} {
				_, err := client.Mint(t.Context(), "secret-jwt", "owner", "repo", id)
				require.Error(t, err)
				require.NotContains(t, err.Error(), "secret-jwt")
				require.NotContains(t, err.Error(), "sensitive-body")
			}
		})
	}
	client := Client{HTTP: &http.Client{Timeout: time.Second}, BaseURL: ":"}
	_, err := client.Mint(t.Context(), "secret", "owner", "repo", "42")
	require.Error(t, err)
}
