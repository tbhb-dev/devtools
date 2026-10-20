// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

// Token is a repository-scoped installation credential and its server expiry.
type Token struct {
	Value   string    `json:"token"`
	Expires time.Time `json:"expires_at"`
}

// Client uses an App JSON Web Token only to discover installations and request tokens.
type Client struct {
	HTTP    *http.Client
	BaseURL string
}

// Mint looks up the repository installation unless installation is configured.
func (c Client) Mint(ctx context.Context, jwt, owner, repo, installation string) (Token, error) {
	if installation == "" {
		var result struct {
			ID int64 `json:"id"`
		}
		if err := c.request(
			ctx,
			http.MethodGet,
			"/repos/"+owner+"/"+repo+"/installation",
			jwt,
			nil,
			&result,
		); err != nil {
			return Token{}, err
		}
		if result.ID <= 0 {
			return Token{}, fmt.Errorf("%w: missing installation id", os.ErrInvalid)
		}
		installation = strconv.FormatInt(result.ID, 10)
	}
	body, err := json.Marshal(struct {
		Repositories []string          `json:"repositories"`
		Permissions  map[string]string `json:"permissions"`
	}{[]string{repo}, map[string]string{"pull_requests": "write", "metadata": "read"}})
	if err != nil {
		return Token{}, fmt.Errorf("encode token request: %w", err)
	}
	var token Token
	err = c.request(ctx, http.MethodPost, "/app/installations/"+installation+"/access_tokens", jwt, body, &token)
	if err != nil {
		return Token{}, err
	}
	if token.Value == "" || token.Expires.IsZero() {
		return Token{}, fmt.Errorf("%w: incomplete installation token", os.ErrInvalid)
	}
	return token, nil
}

func (c Client) request(ctx context.Context, method, path, jwt string, body []byte, result any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create GitHub request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-Github-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.HTTP.Do(req)
	// Don't include request errors or response bodies: either could expose credentials.
	if err != nil {
		return fmt.Errorf("%w: GitHub request failed", os.ErrNotExist)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%w: GitHub returned HTTP %d", os.ErrPermission, response.StatusCode)
	}
	if err = json.NewDecoder(response.Body).Decode(result); err != nil {
		return fmt.Errorf("%w: invalid GitHub response", os.ErrInvalid)
	}
	return nil
}
