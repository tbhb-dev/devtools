// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package apmsync reconciles the hook keys cleared before APM installation.
package apmsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	invalidJSON            = 4 // JSON parse failure.
	invalidDocument        = 5 // A named key requires an object.
	missingExecutable      = 127
	inaccessibleExecutable = 126
)

// Run clears obsolete hooks and installs once, preserving install streams and status.
func Run(ctx context.Context, root string, in io.Reader, out, stderr io.Writer) int {
	if err := reconcile(root); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		if syntax, ok := errors.AsType[*json.SyntaxError](err); ok && syntax != nil {
			return invalidJSON
		}
		if shape, ok := errors.AsType[*json.UnmarshalTypeError](err); ok && shape != nil {
			return invalidDocument
		}
		return 1
	}
	cmd := exec.CommandContext(ctx, "apm", "install")
	cmd.Dir = root
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			return exit.ExitCode()
		}
		_, _ = fmt.Fprintln(stderr, err)
		if errors.Is(err, exec.ErrNotFound) {
			return missingExecutable
		}
		if errors.Is(err, os.ErrPermission) {
			return inaccessibleExecutable
		}
		return 1
	}
	return 0
}

func reconcile(root string) error {
	for _, document := range []struct{ path, key string }{
		{".claude/settings.json", "hooks"},
		{".agents/hooks.json", "apm"},
		{".codex/hooks.json", "hooks"},
	} {
		path := filepath.Join(root, document.path)
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("reconcile hooks: %w", err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err = clean(path, document.key); err != nil {
			return fmt.Errorf("reconcile hooks: %w", err)
		}
	}
	return removeSidecars(root)
}

func removeSidecars(root string) error {
	for _, harness := range []string{".claude", ".agents", ".codex"} {
		path := filepath.Join(root, harness, "apm-hooks.json")
		if info, err := os.Lstat(path); err == nil && info.IsDir() {
			return fmt.Errorf("remove %s: %w", path, fs.ErrInvalid)
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("reconcile hooks: %w", err)
		}
	}
	return nil
}

func clean(path, key string) error {
	// Like the shell redirection, create the temporary file before inspecting settings.
	tmp, err := os.Create(path + ".tmp")
	if err != nil {
		return fmt.Errorf("reconcile hooks: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("reconcile hooks: %w", err)
	}
	var settings map[string]json.RawMessage
	if err = json.Unmarshal(data, &settings); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%s: %w", path, err)
	}
	delete(settings, key)
	data, err = json.MarshalIndent(settings, "", "  ")
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("reconcile hooks: %w", err)
	}
	_, writeErr := tmp.Write(append(data, '\n'))
	closeErr := tmp.Close()
	if writeErr != nil {
		return fmt.Errorf("write settings: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close settings: %w", closeErr)
	}
	if err = os.Rename(path+".tmp", path); err != nil {
		return fmt.Errorf("reconcile hooks: %w", err)
	}
	if len(settings) == 0 {
		if err = os.Remove(path); err != nil {
			return fmt.Errorf("remove empty settings: %w", err)
		}
	}
	return nil
}
