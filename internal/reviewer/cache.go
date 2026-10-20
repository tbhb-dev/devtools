// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package reviewer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tbhb-dev/devtools/internal/ghapp"
)

func cachePath(c Config, repo string) string {
	// Bind cached credentials to the repository and machine's credential configuration.
	digest := sha256.Sum256(
		[]byte(
			repo + "\n" + c.Provider + "\n" + c.AppID + "\n" + c.AppIDReference + "\n" + c.KeySource + "\n" + c.Installation,
		),
	)
	return filepath.Join(c.CacheDir, hex.EncodeToString(digest[:])+".json")
}

func cachedToken(c Config, repo string, now time.Time) (ghapp.Token, bool) {
	dir, err := os.Lstat(c.CacheDir)
	if err != nil || !dir.IsDir() || dir.Mode().Perm() != 0o700 {
		return ghapp.Token{}, false
	}
	path := cachePath(c, repo)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return ghapp.Token{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ghapp.Token{}, false
	}
	var token ghapp.Token
	if json.Unmarshal(data, &token) != nil || token.Value == "" || !now.Before(token.Expires.Add(-5*time.Minute)) {
		return ghapp.Token{}, false
	}
	return token, true
}

func storeToken(c Config, repo string, token ghapp.Token) error {
	if err := os.MkdirAll(c.CacheDir, 0o700); err != nil {
		return fmt.Errorf("create reviewer cache: %w", err)
	}
	info, err := os.Lstat(c.CacheDir)
	if err != nil {
		return fmt.Errorf("inspect reviewer cache: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: cache must be a directory", os.ErrInvalid)
	}
	//nolint:gosec // Private token cache requires owner traversal.
	if err = os.Chmod(c.CacheDir, 0o700); err != nil {
		return fmt.Errorf("protect reviewer cache: %w", err)
	}
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("encode cached token: %w", err)
	}
	file, err := os.CreateTemp(c.CacheDir, ".token-*")
	if err != nil {
		return fmt.Errorf("create cached token: %w", err)
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write cached token: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close cached token: %w", closeErr)
	}
	if err = os.Rename(file.Name(), cachePath(c, repo)); err != nil {
		return fmt.Errorf("publish cached token: %w", err)
	}
	return nil
}
