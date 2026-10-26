// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package bats discovers and runs Bats files with bounded parallelism.
package bats

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
)

// serialFiles is the only serial-only list. Add a reason beside each entry
// only after demonstrating a conflict. These files run after the parallel
// group. The list starts empty.
func serialFiles() map[string]string { return map[string]string{} }

var (
	errJobs  = errors.New("HOOKS_JOBS must be a positive integer")
	errEmpty = errors.New("no Bats files")
)

const maxJobs = 8

// Run runs every discovered file and returns a failing status for invalid
// input, failed discovery, or a failed Bats group.
func Run(ctx context.Context, path, override string, out, diagnostic io.Writer) int {
	cpus := runtime.NumCPU()
	count, err := jobs(override, cpus)
	if err != nil {
		fmt.Fprintln(diagnostic, "test-hooks:", err)
		return 1
	}
	files, err := discover(path)
	if err != nil {
		fmt.Fprintln(diagnostic, "test-hooks:", err)
		return 1
	}
	parallel, serial := partition(files, serialFiles())
	fmt.Fprintf(
		out,
		"test-hooks: %d jobs (%d CPUs), %d parallel files, %d serial files\n",
		count,
		cpus,
		len(parallel),
		len(serial),
	)
	bin, err := os.MkdirTemp("", "test-hooks-")
	if err != nil {
		fmt.Fprintln(diagnostic, "test-hooks:", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(bin) }()
	build := exec.CommandContext(ctx, "go", "install", "./cmd/agentrepo")
	build.Env = append(os.Environ(), "GOBIN="+bin)
	build.Stdout, build.Stderr = out, diagnostic
	if err = build.Run(); err != nil {
		fmt.Fprintln(diagnostic, "test-hooks: building agentrepo:", err)
		return 1
	}
	return execute(parallel, serial, count, func(args []string) int {
		cmd := exec.CommandContext(ctx, "bats", args...)
		cmd.Stdout, cmd.Stderr = out, diagnostic
		cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		if runErr := cmd.Run(); runErr != nil {
			fmt.Fprintln(diagnostic, "test-hooks:", runErr)
			var exit *exec.ExitError
			if errors.As(runErr, &exit) && exit.ExitCode() > 0 {
				return exit.ExitCode()
			}
			return 1
		}
		return 0
	})
}

func jobs(override string, cpus int) (int, error) {
	if override == "" {
		return min(cpus, maxJobs), nil
	}
	count, err := strconv.Atoi(override)
	if err != nil || count < 1 {
		return 0, fmt.Errorf("%w: %q", errJobs, override)
	}
	return min(count, maxJobs), nil
}

func discover(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Ext(path) == ".bats" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discovering %s: %w", root, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w in %s", errEmpty, root)
	}
	slices.Sort(files)
	return files, nil
}

func partition(files []string, serialOnly map[string]string) ([]string, []string) {
	var parallel, serial []string
	for _, file := range files {
		if _, ok := serialOnly[filepath.ToSlash(file)]; ok {
			serial = append(serial, file)
		} else {
			parallel = append(parallel, file)
		}
	}
	return parallel, serial
}

func execute(parallel, serial []string, count int, run func([]string) int) int {
	status := 0
	if len(parallel) > 0 {
		args := parallel
		if count > 1 {
			args = slices.Concat([]string{
				"--jobs", strconv.Itoa(count),
				"--parallel-binary-name", "rush",
				"--no-parallelize-within-files",
			}, args)
		}
		status = run(args)
	}
	if len(serial) > 0 {
		serialStatus := run(serial)
		if status == 0 {
			status = serialStatus
		}
	}
	return status
}
