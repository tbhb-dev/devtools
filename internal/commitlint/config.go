// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package commitlint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config contains commit-message rules and optional identity restrictions.
type Config struct {
	Types               []string   `toml:"types"`
	HeaderMinLength     int        `toml:"header_min_length"`
	HeaderMaxLength     int        `toml:"header_max_length"`
	BodyMaxLineLength   int        `toml:"body_max_line_length"`
	FooterMaxLineLength int        `toml:"footer_max_line_length"`
	RequireSignedOffBy  bool       `toml:"require_signed_off_by"`
	Identities          Identities `toml:"identities"`
}

// DefaultConfig matches the original shell linter defaults.
func DefaultConfig() Config {
	return Config{
		strings.Fields("feat fix docs style refactor perf test build ci chore revert"),
		10, 80, 72, 100, true,
		Identities{},
	}
}

// LoadConfig finds the repository config by walking to the nearest Git root.
// A config in a parent repository can't affect a nested repository.
func LoadConfig(dir string, getenv func(string) string) (Config, error) {
	c := DefaultConfig()
	if err := readConfig(dir, &c); err != nil {
		return c, err
	}
	if err := overrideConfig(&c, getenv); err != nil {
		return c, err
	}
	if c.HeaderMinLength < 0 || c.HeaderMaxLength < c.HeaderMinLength || c.BodyMaxLineLength < 0 ||
		c.FooterMaxLineLength < 0 ||
		len(c.Types) == 0 {
		return c, errInvalidConfig
	}
	for _, list := range [][]Identity{c.Identities.Agents, c.Identities.SignOff} {
		for _, identity := range list {
			if strings.TrimSpace(identity.Email) == "" {
				return c, errEmptyIdentityEmail
			}
		}
	}
	return c, nil
}

var errInvalidConfig = errors.New("invalid commit-message bounds or empty types")

func readConfig(dir string, c *Config) error {
	for {
		data, err := os.ReadFile(filepath.Join(dir, ".commit-lint.toml"))
		if err == nil {
			if err = toml.Unmarshal(data, c); err != nil {
				return fmt.Errorf(".commit-lint.toml: %w", err)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read .commit-lint.toml: %w", err)
		}
		_, gitErr := os.Stat(filepath.Join(dir, ".git"))
		parent := filepath.Dir(dir)
		if gitErr == nil || parent == dir {
			break
		}
		dir = parent
	}
	return nil
}

func overrideConfig(c *Config, getenv func(string) string) error {
	for _, setting := range []struct {
		name   string
		target *int
	}{
		{"HEADER_MIN_LENGTH", &c.HeaderMinLength},
		{"HEADER_MAX_LENGTH", &c.HeaderMaxLength},
		{"BODY_MAX_LINE_LENGTH", &c.BodyMaxLineLength},
		{"FOOTER_MAX_LINE_LENGTH", &c.FooterMaxLineLength},
	} {
		if value := getenv("COMMITLINT_" + setting.name); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("COMMITLINT_%s: expected an integer: %w", setting.name, err)
			}
			*setting.target = n
		}
	}
	if value := getenv("COMMITLINT_TYPES"); value != "" {
		c.Types = strings.Fields(value)
	}
	if value := getenv("COMMITLINT_REQUIRE_SIGNED_OFF_BY"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("COMMITLINT_REQUIRE_SIGNED_OFF_BY: expected an integer: %w", err)
		}
		c.RequireSignedOffBy = n == 1
	}
	return nil
}

var errEmptyIdentityEmail = errors.New("identity email pattern must not be empty")
