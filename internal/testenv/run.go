// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package testenv isolates command configuration while retaining tool stores.
package testenv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const (
	stockPath  = "/usr/bin:/bin:/usr/sbin:/sbin"
	usageError = 2
)

// Run executes a command with a temporary home and configuration.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: testenv command [args...]")
		return usageError
	}
	if err := run(args, stdin, stdout, stderr); err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(stderr, "testenv:", err)
		return 1
	}
	return 0
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	rootBytes, err := exec.CommandContext(context.Background(), "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("find repository: %w", err)
	}
	root := strings.TrimSpace(string(rootBytes))
	mise, err := exec.LookPath("mise")
	if err != nil {
		return fmt.Errorf("prepare test environment: %w", err)
	}
	// Resolve shims so their sibling tool shims can't enter the clean PATH.
	mise, err = filepath.EvalSymlinks(mise)
	if err != nil {
		return fmt.Errorf("prepare test environment: %w", err)
	}
	home, err := os.MkdirTemp("", "testenv-")
	if err != nil {
		return fmt.Errorf("prepare test environment: %w", err)
	}
	defer os.RemoveAll(home)
	config := filepath.Join(home, "config.toml")
	if err = os.WriteFile(config, nil, 0o600); err != nil {
		return fmt.Errorf("prepare test environment: %w", err)
	}
	stores, err := toolStores(os.Environ())
	if err != nil {
		return fmt.Errorf("prepare test environment: %w", err)
	}
	env := environment(os.Environ(), stores, root, home, config, mise, nil)
	trust := exec.CommandContext(context.Background(), mise, "trust", root)
	trust.Dir, trust.Env, trust.Stderr = root, env, stderr
	if err = trust.Run(); err != nil {
		return fmt.Errorf("trust repository: %w", err)
	}
	paths := exec.CommandContext(context.Background(), mise, "bin-paths")
	paths.Dir, paths.Env, paths.Stderr = root, env, stderr
	output, err := paths.Output()
	if err != nil {
		return fmt.Errorf("mise bin-paths: %w", err)
	}
	env = environment(
		os.Environ(),
		stores,
		root,
		home,
		config,
		mise,
		strings.FieldsFunc(string(output), func(r rune) bool { return r == '\n' || r == '\r' }),
	)
	program, err := findCommand(args[0], env)
	if err != nil {
		return fmt.Errorf("prepare test environment: %w", err)
	}
	cmd := exec.CommandContext(context.Background(), program, args[1:]...)
	cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = env, stdin, stdout, stderr
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("run command: %w", err)
	}
	return nil
}

func toolStores(inherited []string) (map[string]string, error) {
	values := envMap(inherited)
	home := values["HOME"]
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("find cache directory: %w", err)
	}
	data := values["XDG_DATA_HOME"]
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	defaults := map[string]string{
		"MISE_DATA_DIR":  filepath.Join(data, "mise"),
		"MISE_CACHE_DIR": filepath.Join(cache, "mise"),
		"CARGO_HOME":     filepath.Join(home, ".cargo"),
		"RUSTUP_HOME":    filepath.Join(home, ".rustup"),
	}
	output, err := exec.CommandContext(context.Background(), "go", "env", "GOMODCACHE", "GOCACHE").Output()
	if err != nil {
		return nil, fmt.Errorf("find Go stores: %w", err)
	}
	goPaths := strings.Split(strings.TrimSpace(string(output)), "\n")
	defaults["GOMODCACHE"], defaults["GOCACHE"] = goPaths[0], goPaths[1]
	for key := range defaults {
		if values[key] != "" {
			defaults[key] = values[key]
		}
	}
	return defaults, nil
}

func environment(
	inherited []string,
	stores map[string]string,
	root, home, config, mise string,
	bins []string,
) []string {
	values := make(map[string]string)
	for key, value := range envMap(inherited) {
		if key == "TERM" || key == "TMPDIR" || key == "CI" || strings.HasPrefix(key, "GITHUB_") ||
			strings.HasPrefix(key, "DEVTOOLS_") {
			values[key] = value
		}
	}
	maps.Copy(values, stores)
	values["HOME"] = home
	values["GIT_CONFIG_GLOBAL"] = "/dev/null"
	values["GIT_CONFIG_NOSYSTEM"] = "1"
	values["MISE_GLOBAL_CONFIG_FILE"] = config
	values["MISE_CEILING_PATHS"] = filepath.Dir(root)
	paths := append([]string{stockPath, filepath.Join(root, "bin"), filepath.Dir(mise)}, bins...)
	if bin := filepath.Join(stores["CARGO_HOME"], "bin"); isDirectory(bin) {
		paths = append(paths, bin)
	}
	values["PATH"] = strings.Join(paths, string(os.PathListSeparator))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}

func envMap(env []string) map[string]string {
	values := make(map[string]string)
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		values[key] = value
	}
	return values
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// exec.LookPath uses the process environment, so resolve against the child's PATH.
func findCommand(name string, env []string) (string, error) {
	if strings.ContainsRune(name, filepath.Separator) {
		return name, nil
	}
	for _, dir := range filepath.SplitList(envMap(env)["PATH"]) {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return path, nil
		}
	}
	return "", fmt.Errorf("command %q: %w", name, exec.ErrNotFound)
}
