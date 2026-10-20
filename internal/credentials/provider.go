// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package credentials

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// OPCommand constructs an op reader with an optional operator-owned token broker.
// requireServiceAccount prevents op from falling back to interactive authentication.
func OPCommand(ctx context.Context, reference, tokenCommand string, requireServiceAccount bool) (*exec.Cmd, error) {
	token := os.Getenv("OP_SERVICE_ACCOUNT_TOKEN")
	if tokenCommand != "" {
		output, err := exec.CommandContext(ctx, tokenCommand).Output()
		if err != nil {
			return nil, fmt.Errorf("read provider token: %w", err)
		}
		token = strings.TrimSpace(string(output))
	}
	if requireServiceAccount && token == "" {
		return nil, fmt.Errorf("%w: service account token or TOKEN_COMMAND required", os.ErrInvalid)
	}
	cmd := exec.CommandContext(ctx, "op", "read", reference)
	cmd.Env = ReplaceEnv(os.Environ(), "OP_SERVICE_ACCOUNT_TOKEN", token)
	return cmd, nil
}

// ReplaceEnv replaces every inherited occurrence of a variable.
func ReplaceEnv(env []string, key, value string) []string {
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, key+"=") {
			result = append(result, entry)
		}
	}
	return append(result, key+"="+value)
}

// Read reads a file or a service-account-backed op reference into memory.
func Read(ctx context.Context, provider, source, tokenCommand string) ([]byte, error) {
	if provider == "file" {
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("read credential file: %w", err)
		}
		return data, nil
	}
	cmd, err := OPCommand(ctx, source, tokenCommand, true)
	if err != nil {
		return nil, err
	}
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("read provider: %w", err)
	}
	return data, nil
}
