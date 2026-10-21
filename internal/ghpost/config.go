// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/tbhb-dev/devtools/internal/credentials"
)

const (
	requiredList    = "required"
	secretsOnlyMode = "secrets-only"
)

var errConfig = errors.New("cannot load machine configuration or required private strings")

// settings uses the target repository name, never the checkout's name.
func settings(repo string) (map[string]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errConfig
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(home, ".config")
	}
	result := map[string]string{}
	for _, file := range []string{"devtools.conf", strings.Split(repo, "/")[1] + ".conf"} {
		if err = readSettings(filepath.Join(dir, "devtools", file), result); err != nil {
			return nil, err
		}
	}

	for _, key := range []string{"private_strings_ref", "private_strings_token_command", "scan_mode", "post_identities"} {
		if value, ok := os.LookupEnv("DEVTOOLS_" + strings.ToUpper(key)); ok {
			result[key] = value
		}
	}
	return result, nil
}

func privateStrings(ctx context.Context, config map[string]string) ([]string, error) {
	// The exported list takes precedence over machine policy, including none.
	if value, ok := os.LookupEnv("DEVTOOLS_PRIVATE_STRINGS"); ok {
		list := splitList(value)
		if len(list) == 0 && (config["private_strings_ref"] != "" || config["private_strings"] == requiredList) {
			return nil, errConfig
		}
		return list, nil
	}
	if config["private_strings"] == "none" {
		return nil, nil
	}
	ref := config["private_strings_ref"]
	if ref == "" {
		if config["private_strings"] == requiredList {
			return nil, errConfig
		}
		return nil, nil
	}
	data, err := credentials.Read(ctx, "op", ref, config["private_strings_token_command"])
	if err != nil {
		return nil, errConfig
	}
	list := splitList(string(data))
	if len(list) == 0 {
		return nil, errConfig
	}
	return list, nil
}

func splitList(value string) []string {
	var result []string
	for part := range strings.SplitSeq(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func readSettings(path string, result map[string]string) error {
	// The path comes from machine configuration and a validated repository name.
	//nolint:gosec // Only operator-owned config and validated repository names select this path.
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errConfig
	}
	seen := map[string]bool{}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if ok && !seen[key] {
			result[key] = strings.TrimSpace(value)
			seen[key] = true
		}
	}
	return nil
}
