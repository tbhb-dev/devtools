// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package reviewer

import (
	"debug/buildinfo"
	"fmt"
	"os"
	"path/filepath"
)

// Install replaces only devtools gh-reviewer binaries unless the operator opts in.
func Install(home, binary string, force bool) error {
	dir := filepath.Join(home, ".local/bin")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create binary directory: %w", err)
	}
	target := filepath.Join(dir, "gh-reviewer")
	if _, err := os.Lstat(target); err == nil {
		info, readErr := buildinfo.ReadFile(target)
		if !force && (readErr != nil || info.Path != "github.com/tbhb-dev/devtools/cmd/gh-reviewer") {
			return fmt.Errorf("%w: existing gh-reviewer is not a devtools binary; use --force", os.ErrExist)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect installed reviewer: %w", err)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		return fmt.Errorf("read reviewer binary: %w", err)
	}
	file, err := os.CreateTemp(dir, ".gh-reviewer-*")
	if err != nil {
		return fmt.Errorf("create reviewer binary: %w", err)
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write reviewer binary: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close reviewer binary: %w", closeErr)
	}
	//nolint:gosec // The installed binary must be executable by its owner.
	if err = os.Chmod(file.Name(), 0o700); err != nil {
		return fmt.Errorf("make reviewer executable: %w", err)
	}
	if err = os.Rename(file.Name(), target); err != nil {
		return fmt.Errorf("install reviewer binary: %w", err)
	}
	return nil
}
