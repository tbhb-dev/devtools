// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type auditRecord struct {
	Time      string `json:"time"`
	Operation string `json:"operation"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
}

func openAudit() (*os.File, error) {
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("audit home: %w", err)
		}
		state = filepath.Join(home, ".local", "state")
	}
	dir := filepath.Join(state, "devtools")
	//nolint:gosec // The operator chooses the state directory; it is not posted content.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("audit directory: %w", err)
	}
	//nolint:gosec // The audit path is fixed beneath the operator-owned state directory.
	file, err := os.OpenFile(filepath.Join(dir, "gh-post.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit file: %w", err)
	}
	return file, nil
}

func appendAudit(out io.Writer, operation, target string, body []byte) error {
	hash := sha256.Sum256(body)
	record := auditRecord{time.Now().UTC().Format(time.RFC3339Nano), operation, target, hex.EncodeToString(hash[:])}
	if err := json.NewEncoder(out).Encode(record); err != nil {
		return fmt.Errorf("audit append: %w", err)
	}
	return nil
}
