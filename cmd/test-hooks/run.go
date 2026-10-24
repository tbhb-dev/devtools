// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
)

// serialFiles lists repository-relative paths with demonstrated parallel
// conflicts. Add the reason beside each entry. No conflicts are known.
func serialFiles() []string { return []string{} }

const maxJobs = 8

// File workers are capped at eight to bound process and memory use. Bats
// keeps tests within each file serial and creates their temporary directories.
func runSuite(root string, cpuCount int, serial []string, run func([]string) error) error {
	var parallelPaths, serialPaths []string
	err := filepath.WalkDir(filepath.Join(root, "tests"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".bats" {
			return nil
		}
		if slices.ContainsFunc(serial, func(file string) bool { return path == filepath.Join(root, file) }) {
			serialPaths = append(serialPaths, path)
		} else {
			parallelPaths = append(parallelPaths, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("discover hook suites: %w", err)
	}
	var result error
	if len(parallelPaths) > 0 {
		jobs := min(cpuCount, maxJobs)
		args := []string{"--jobs", strconv.Itoa(jobs)}
		if jobs > 1 {
			args = append(args, "--no-parallelize-within-files", "--parallel-binary-name", "rush")
		}
		result = run(append(args, parallelPaths...))
	}
	if len(serialPaths) > 0 {
		result = errors.Join(result, run(append([]string{"--jobs", "1"}, serialPaths...)))
	}
	return result
}
