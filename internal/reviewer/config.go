// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package reviewer runs GitHub CLI with repository-scoped App credentials.
package reviewer

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/tbhb-dev/devtools/internal/credentials"
)

// Config describes the machine's App credential provider.
type Config struct {
	Provider       string
	AppID          string
	AppIDReference string
	KeySource      string
	TokenCommand   string
	Installation   string
	CacheDir       string
}

// ReadConfig shares signing's literal assignment format and environment precedence.
func ReadConfig(home string, getenv func(string) string) (Config, error) {
	const prefix = "DEVTOOLS_REVIEWER_"
	values := map[string]string{"PROVIDER": "op"}
	path := getenv(prefix + "CONFIG")
	if path == "" {
		path = filepath.Join(home, ".config/devtools/reviewer.env")
	}
	if err := credentials.ReadSettings(path, prefix, values); err != nil {
		return Config{}, fmt.Errorf("reviewer config: %w", err)
	}
	for _, key := range []string{"PROVIDER", "APP_ID", "APP_ID_REFERENCE", "OP_REFERENCE", "KEY_FILE", "TOKEN_COMMAND", "INSTALLATION_ID"} {
		if value := getenv(prefix + key); value != "" {
			values[key] = value
		}
	}
	c := Config{
		Provider:       values["PROVIDER"],
		AppID:          values["APP_ID"],
		AppIDReference: values["APP_ID_REFERENCE"],
		TokenCommand:   values["TOKEN_COMMAND"],
		Installation:   values["INSTALLATION_ID"],
		CacheDir:       filepath.Join(home, ".local/state/devtools/reviewer"),
	}
	switch c.Provider {
	case "op":
		c.KeySource = values["OP_REFERENCE"]
		if c.AppIDReference == "" {
			return Config{}, fmt.Errorf("%w: APP_ID_REFERENCE required for op", os.ErrInvalid)
		}
	case "file":
		c.KeySource = values["KEY_FILE"]
		if !filepath.IsAbs(c.KeySource) || c.AppID == "" {
			return Config{}, fmt.Errorf("%w: file provider requires APP_ID and absolute KEY_FILE", os.ErrInvalid)
		}
	default:
		return Config{}, fmt.Errorf("%w: PROVIDER must be op or file", os.ErrInvalid)
	}
	if c.KeySource == "" {
		return Config{}, fmt.Errorf("%w: private key source required", os.ErrInvalid)
	}
	if err := validateInstallation(c.Installation); err != nil {
		return Config{}, err
	}
	return c, nil
}

func validateInstallation(value string) error {
	if value == "" {
		return nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("%w: INSTALLATION_ID must be positive", os.ErrInvalid)
	}
	return nil
}
