// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package signing manages an isolated, short-lived SSH signing identity.
package signing

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config describes the dedicated agent and its private-key provider.
type Config struct {
	RunDir       string
	PublicKey    string
	Provider     string
	KeyFile      string
	OPReference  string
	TokenCommand string
	Hook         string
	TTL          time.Duration
	Reload       time.Duration
}

// ReadConfig reads data-only assignments, then applies environment overrides.
func ReadConfig(home string, getenv func(string) string) (Config, error) {
	values := map[string]string{
		"RUN_DIR":    filepath.Join(home, ".local/state/devtools/signing"),
		"PUBLIC_KEY": filepath.Join(home, ".config/devtools/signing-key.pub"),
		"PROVIDER":   "op", "KEY_TTL": "3600", "RELOAD": "2700",
	}
	path := getenv("DEVTOOLS_SIGNING_CONFIG")
	if path == "" {
		path = filepath.Join(home, ".config/devtools/signing.env")
	}
	if err := readSettings(path, values); err != nil {
		return Config{}, err
	}
	for _, key := range []string{"RUN_DIR", "PUBLIC_KEY", "PROVIDER", "KEY_FILE", "OP_REFERENCE", "TOKEN_COMMAND", "HOOK", "KEY_TTL", "RELOAD"} {
		if value := getenv("DEVTOOLS_SIGNING_" + key); value != "" {
			values[key] = value
		}
	}
	ttl, err := strconv.ParseInt(values["KEY_TTL"], 10, 32)
	if err != nil {
		return Config{}, fmt.Errorf("KEY_TTL: %w", err)
	}
	reload, err := strconv.ParseInt(values["RELOAD"], 10, 32)
	if err != nil {
		return Config{}, fmt.Errorf("RELOAD: %w", err)
	}
	if ttl <= 0 || reload <= 0 || reload >= ttl {
		return Config{}, fmt.Errorf("%w: require 0 < RELOAD < KEY_TTL", os.ErrInvalid)
	}
	if values["PROVIDER"] != "op" && values["PROVIDER"] != "file" {
		return Config{}, fmt.Errorf("%w: PROVIDER must be op or file", os.ErrInvalid)
	}
	return Config{
		RunDir:       values["RUN_DIR"],
		PublicKey:    values["PUBLIC_KEY"],
		Provider:     values["PROVIDER"],
		KeyFile:      values["KEY_FILE"],
		OPReference:  values["OP_REFERENCE"],
		TokenCommand: values["TOKEN_COMMAND"],
		Hook:         values["HOOK"],
		TTL:          time.Duration(ttl) * time.Second,
		Reload:       time.Duration(reload) * time.Second,
	}, nil
}

func readSettings(path string, values map[string]string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read signing config: %w", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.HasPrefix(key, "DEVTOOLS_SIGNING_") {
			return fmt.Errorf("%w: signing config expects DEVTOOLS_SIGNING_NAME=value", os.ErrInvalid)
		}
		values[strings.TrimPrefix(key, "DEVTOOLS_SIGNING_")] = value
	}
	if err = scanner.Err(); err != nil {
		return fmt.Errorf("scan signing config: %w", err)
	}
	return nil
}
