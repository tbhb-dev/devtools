// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package config reads literal skill settings, separating machine authority
// from repository behavior. Configuration is never evaluated as shell code.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Lookup discovers Git context lazily: an environment override works even
// outside a repository, while file lookup requires successful discovery.
type Lookup struct {
	LookupEnv      func(string) (string, bool)
	RepositoryName func() (string, error)
	RepositoryRoot func() (string, error)
}

// Machine resolves DEVTOOLS_KEY, the machine's repository file, its shared
// defaults, then fallback. An explicitly empty value overrides later sources.
func (l Lookup) Machine(key, fallback string) (string, error) {
	if value, ok := l.LookupEnv("DEVTOOLS_" + strings.ToUpper(key)); ok {
		return value, nil
	}
	name, err := l.RepositoryName()
	if err != nil {
		return "", fmt.Errorf("repository name: %w", err)
	}
	dir, _ := l.LookupEnv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := l.LookupEnv("HOME")
		dir = home + "/.config"
	}
	for _, file := range []string{name + ".conf", "devtools.conf"} {
		path := filepath.Join(dir, "devtools", file)
		info, statErr := os.Stat(path)
		if statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", fmt.Errorf("read machine configuration: %w", readErr)
		}
		if value, ok := firstValue(string(data), key); ok {
			return value, nil
		}
	}
	return fallback, nil
}

// Repository validates the whole behavior file before returning any value.
// The allowlist excludes all machine authorities, even for unrelated lookups.
func (l Lookup) Repository(key, fallback string) (string, error) {
	root, err := l.RepositoryRoot()
	if err != nil {
		return "", fmt.Errorf("repository root: %w", err)
	}
	path := filepath.Join(root, ".agents", "agent-skills.conf")
	// Match the shell's -e: an absent target (including a dangling symlink)
	// has no settings. Existing symlinks and other non-regular files refuse.
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return fallback, nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("stat repository configuration: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s", errRegularFile, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read repository configuration: %w", err)
	}
	if err = validate(string(data)); err != nil {
		return "", err
	}
	if value, ok := firstValue(string(data), key); ok {
		return value, nil
	}
	return fallback, nil
}

func assignment(line string) (string, string, bool) {
	key, value, ok := strings.Cut(line, "=")
	return strings.Trim(key, " \t"), strings.Trim(value, " \t"), ok
}

func firstValue(data, key string) (string, bool) {
	for line := range strings.SplitSeq(data, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
			continue
		}
		k, value, ok := assignment(line)
		if ok && k == key {
			return value, true
		}
	}
	return "", false
}
