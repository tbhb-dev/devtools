// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package credentials shares machine configuration and private-key providers.
package credentials

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ReadSettings reads literal prefixed assignments into values.
func ReadSettings(path, prefix string, values map[string]string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.HasPrefix(key, prefix) {
			return fmt.Errorf("%w: config expects %sNAME=value", os.ErrInvalid, prefix)
		}
		values[strings.TrimPrefix(key, prefix)] = value
	}
	if err = scanner.Err(); err != nil {
		return fmt.Errorf("scan config: %w", err)
	}
	return nil
}
