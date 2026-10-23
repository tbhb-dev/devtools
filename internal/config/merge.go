// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var errMergeEnvironment = errors.New("merge authority refuses environment variable")

// MergeMachine refuses environment authority and reads only machine files.
func (l Lookup) MergeMachine(key, fallback string) (string, error) {
	for _, authority := range []string{"merge_owner", "required_reviewer", "required_reviewer_type", "delegation_check", "gh_login"} {
		name := "DEVTOOLS_" + strings.ToUpper(authority)
		if _, set := l.LookupEnv(name); set {
			return "", fmt.Errorf("%w %s; use machine configuration", errMergeEnvironment, name)
		}
	}
	return l.machineFile(key, fallback)
}

// RepositoryAt validates the pinned base's settings and reports checkout drift.
// The checkout never supplies values, even when its copy is valid and committed.
func (l Lookup) RepositoryAt(ctx context.Context, base, key, fallback string) (string, bool, error) {
	root, err := l.RepositoryRoot()
	if err != nil {
		return "", false, fmt.Errorf("repository root: %w", err)
	}
	const path = ".agents/agent-skills.conf"
	// Fixed read-only Git operations. Base is a separate revision argument.
	tree, err := exec.CommandContext(ctx, "git", "-C", root, "ls-tree", base, "--", path).Output()
	if err != nil {
		return "", false, fmt.Errorf("read pinned repository configuration: %w", err)
	}
	var data []byte
	if len(tree) > 0 {
		if !bytes.HasPrefix(tree, []byte("100644 blob ")) && !bytes.HasPrefix(tree, []byte("100755 blob ")) {
			return "", false, fmt.Errorf("%w: %s", errRegularFile, path)
		}
		data, err = exec.CommandContext(ctx, "git", "-C", root, "show", base+":"+path).Output()
		if err != nil {
			return "", false, fmt.Errorf("read pinned repository configuration: %w", err)
		}
	}
	if err = validate(string(data)); err != nil {
		return "", false, err
	}
	checkout, readErr := os.ReadFile(filepath.Join(root, path))
	differs := !bytes.Equal(data, checkout) || (readErr != nil && !os.IsNotExist(readErr))
	if value, ok := firstValue(string(data), key); ok {
		return value, differs, nil
	}
	return fallback, differs, nil
}
