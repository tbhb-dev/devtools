// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package drift regenerates dependencies and compares them to the Git index.
package drift

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type runner func(context.Context, string, []string, []string, io.Writer, io.Writer) int

// Run checks Go dependencies, or the vendir payload when vendir is true.
// Only vendir's environment is scrubbed: Git must read the caller's index.
func Run(ctx context.Context, vendir bool, out, stderr io.Writer) int {
	return check(ctx, vendir, func(ctx context.Context, name string, args, env []string, out, stderr io.Writer) int {
		return execute(ctx, "", name, args, env, out, stderr)
	}, out, stderr)
}

func check(ctx context.Context, vendir bool, run runner, out, stderr io.Writer) int {
	env := os.Environ()
	if !vendir {
		return regenerateGo(ctx, run, env, out, stderr)
	}
	if code := run(ctx, "vendir", []string{"sync"}, syncEnvironment(env), out, stderr); code != 0 {
		return code
	}
	return compare(ctx, true, run, env, out, stderr)
}

func regenerateGo(ctx context.Context, run runner, env []string, out, stderr io.Writer) int {
	var tracked bytes.Buffer
	if code := run(ctx, "git", []string{"ls-files", "--", "vendor/modules.txt"}, env, &tracked, stderr); code != 0 {
		return code
	}
	if strings.TrimRight(tracked.String(), "\n") == "" {
		_, _ = fmt.Fprintln(out, "vendor-check: skipping module without tracked vendor/modules.txt")
		return 0
	}
	for _, operation := range []string{"tidy", "vendor"} {
		if code := run(ctx, "go", []string{"mod", operation}, env, out, stderr); code != 0 {
			return code
		}
	}
	return compare(ctx, false, run, env, out, stderr)
}

func compare(ctx context.Context, vendir bool, run runner, env []string, out, stderr io.Writer) int {
	diff := []string{"diff", "--exit-code", "--", "go.mod", "go.sum", "vendor/"}
	scan := []string{"go.mod", "go.sum", "vendor/"}
	message := "vendor drift detected — run 'mise run devtools:vendor' and commit"
	if vendir {
		diff = []string{
			"diff",
			"--stat",
			"--exit-code",
			"--",
			"vendir.yml",
			"vendir.lock.yml",
			".config/mise/conf.d",
			".devtools",
		}
		scan = []string{".config/mise/conf.d", ".devtools"}
		message = "vendored payload drifted from what git holds -- review the resync above, then stage and commit it or fix vendir.yml"
	}
	drift := run(ctx, "git", diff, env, out, stderr) != 0
	var untracked bytes.Buffer
	args := append([]string{"ls-files", "--others", "--exclude-per-directory=.gitignore", "--"}, scan...)
	if code := run(ctx, "git", args, env, &untracked, stderr); code != 0 {
		return code
	}
	names := strings.TrimRight(untracked.String(), "\n")
	if names != "" {
		drift = true
		if vendir {
			// The frozen shell expands the unquoted listing with default whitespace separators.
			for _, name := range shellNames(names) {
				_, _ = fmt.Fprintf(out, "?? %s\n", name)
			}
		} else {
			_, _ = fmt.Fprintln(out, names)
		}
	}
	if drift {
		_, _ = fmt.Fprintln(stderr, message)
		return 1
	}
	if vendir {
		_, _ = fmt.Fprintln(out, "vendored payload matches what git holds")
	}
	return 0
}

// The frozen vendir gate expands its listing without quotes. Keep its default
// whitespace splitting and path expansion, including unmatched patterns.
func shellNames(listing string) []string {
	var names []string
	for name := range strings.FieldsFuncSeq(listing, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' }) {
		pattern := name
		if runtime.GOOS != "windows" {
			pattern = strings.ReplaceAll(pattern, `\`, `\\`)
		}
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			names = append(names, name)
		} else {
			names = append(names, matches...)
		}
	}
	return names
}

func syncEnvironment(env []string) []string {
	clean := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "GIT_DIR", "GIT_COMMON_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY":
		default:
			clean = append(clean, entry)
		}
	}
	return clean
}

const (
	commandNotFound      = 127
	commandNotExecutable = 126
)

func execute(ctx context.Context, dir, name string, args, env []string, out, stderr io.Writer) int {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = out
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			return exit.ExitCode()
		}
		_, _ = fmt.Fprintln(stderr, err)
		if errors.Is(err, exec.ErrNotFound) {
			return commandNotFound
		}
		return commandNotExecutable
	}
	return 0
}
