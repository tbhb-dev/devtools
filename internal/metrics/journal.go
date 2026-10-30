// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package metrics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const (
	maxLineBytes = 4096
	nanoTime     = "2006-01-02T15:04:05.000000000Z07:00"
)

var errTooLarge = errors.New("encoded event line exceeds 4096 bytes")

type eventRecord struct {
	EventName string            `json:"event_name"`
	EventID   string            `json:"event_id"`
	Time      string            `json:"time"`
	Host      string            `json:"host"`
	ItemRef   string            `json:"item_ref,omitempty"`
	Attrs     map[string]string `json:"attrs"`
}

func journalDir(configPath string) (string, error) {
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return "", fmt.Errorf("read config: %w", err)
		}
		var config struct {
			JournalDir string `toml:"journal_dir"`
		}
		if err = toml.Unmarshal(data, &config); err != nil {
			return "", fmt.Errorf("decode config: %w", err)
		}
		if config.JournalDir != "" {
			return config.JournalDir, nil
		}
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find home directory: %w", err)
		}
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "afakt", "journal"), nil
}

func appendEvent(dir, name, itemRef string, attrs map[string]string) error {
	host, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("read hostname: %w", err)
	}
	now := time.Now().UTC()
	stamp := now.Format(nanoTime)
	id := sha256.Sum256(fmt.Appendf(nil, "%s\x00%s\x00%s\x00%s\x00%d", host, name, itemRef, stamp, os.Getpid()))
	line, err := json.Marshal(eventRecord{
		EventName: name,
		EventID:   hex.EncodeToString(id[:]),
		Time:      stamp,
		Host:      host,
		ItemRef:   itemRef,
		Attrs:     attrs,
	})
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	line = append(line, '\n')
	if len(line) > maxLineBytes {
		return errTooLarge
	}
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create journal directory: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(dir, now.Format(time.DateOnly)+".jsonl"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open journal: %w", err)
	}
	defer file.Close()
	// One O_APPEND write keeps concurrent local writers' records together.
	n, err := file.Write(line)
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	if n != len(line) {
		return fmt.Errorf("append event: %w", io.ErrShortWrite)
	}
	return nil
}
