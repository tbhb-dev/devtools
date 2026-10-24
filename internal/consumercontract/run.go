// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrCommand reports a subprocess that couldn't run to a verdict.
var ErrCommand = errors.New("command failed")

// baseEnvEntries counts the variables `hermeticEnv` always sets.
const baseEnvEntries = 11

// hermeticEnv strips every variable Git and the readers consult. A probe then
// reads only the scratch tree, never the host's repository or identity.
func hermeticEnv(home string, extra ...string) []string {
	env := make([]string, 0, baseEnvEntries+len(extra))
	env = append(env,
		"PATH="+os.Getenv("PATH"),
		"HOME="+home,
		"XDG_CONFIG_HOME="+home+"/.config",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=probe",
		"GIT_AUTHOR_EMAIL=probe@example.invalid",
		"GIT_COMMITTER_NAME=probe",
		"GIT_COMMITTER_EMAIL=probe@example.invalid",
		"UV_CACHE_DIR="+cacheDir(),
	)
	return append(env, extra...)
}

// cacheDir keeps uv's cache on the host, so a probe doesn't download packages again.
func cacheDir() string {
	if dir := os.Getenv("UV_CACHE_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir() + "/uv-cache"
	}
	return home + "/.cache/uv"
}

// result is a finished subprocess: its output and whether it exited zero.
type result struct {
	stdout string
	ok     bool
	code   int
}

// run starts a program with an argv vector and no shell. A non-zero exit is
// a verdict and returns no error, because a reader rejecting a state exits
// non-zero.
func run(ctx context.Context, dir string, env []string, stdin, name string, args ...string) (result, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return result{stdout: stdout.String(), ok: true}, nil
	case errors.As(err, &exit):
		return result{stdout: stdout.String(), code: exit.ExitCode()}, nil
	default:
		return result{}, fmt.Errorf("%w: %s: %w", ErrCommand, name, err)
	}
}

// git runs a Git command in dir and fails on a non-zero exit.
func git(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	res, err := run(ctx, dir, env, "", "git", args...)
	if err != nil {
		return "", err
	}
	if !res.ok {
		return "", fmt.Errorf("%w: git %s", ErrCommand, strings.Join(args, " "))
	}
	return res.stdout, nil
}
