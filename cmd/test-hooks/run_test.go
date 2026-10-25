// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"a.bats", "nested/b.bats", "nested/ignored.txt", "serial.bats"} {
		path := filepath.Join(root, "tests", name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, nil, 0o600))
	}
	return root
}

func TestWholeSuiteAndCPUCap(t *testing.T) {
	t.Parallel()
	for _, cpuCount := range []int{1, 4, 8, 32} {
		t.Run(strconv.Itoa(cpuCount), func(t *testing.T) {
			t.Parallel()
			root := fixture(t)
			var calls [][]string
			run := func(args []string) error { calls = append(calls, args); return nil }
			for range 2 {
				require.NoError(t, runSuite(root, cpuCount, nil, run))
			}
			jobs := min(cpuCount, 8)
			want := []string{"--jobs", strconv.Itoa(jobs)}
			if jobs > 1 {
				want = append(want, "--no-parallelize-within-files", "--parallel-binary-name", "rush")
			}
			want = append(
				want,
				filepath.Join(root, "tests/a.bats"),
				filepath.Join(root, "tests/nested/b.bats"),
				filepath.Join(root, "tests/serial.bats"),
			)
			require.Equal(t, [][]string{want, want}, calls)
		})
	}
}

func TestSerialExceptionsFollowParallelEvenOnFailure(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	failed := os.ErrInvalid
	var calls [][]string
	err := runSuite(root, 4, []string{"tests/serial.bats"}, func(args []string) error {
		calls = append(calls, args)
		return failed
	})
	require.ErrorIs(t, err, failed)
	require.Len(t, calls, 2)
	require.Equal(
		t,
		[]string{filepath.Join(root, "tests/a.bats"), filepath.Join(root, "tests/nested/b.bats")},
		calls[0][5:],
	)
	require.Equal(t, []string{"--jobs", "1", filepath.Join(root, "tests/serial.bats")}, calls[1])
	require.Empty(t, serialFiles(), "exceptions require a demonstrated conflict and an adjacent reason")
}

func TestMissingSuiteFails(t *testing.T) {
	t.Parallel()
	require.Error(t, runSuite(t.TempDir(), 4, nil, func([]string) error {
		t.Fatal("must not run an absent suite")
		return nil
	}))
}

func TestBatsFileIsolationAndResults(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bats runs on Linux and macOS in test-bats")
	}
	binDir := t.TempDir()
	// Model Bats' file directories and output without requiring Bats or its scheduler.
	stub := `#!/bin/sh
set -eu
[ "$1" = --jobs ]
jobs=$2
shift 2
if [ "$jobs" -gt 1 ]; then
	[ "$1" = --no-parallelize-within-files ]
	[ "$2" = --parallel-binary-name ]
	[ "$3" = rush ]
	shift 3
fi
printf '%s' "$jobs" >"$RECORD_DIR/jobs"
printf '1..%s\n' "$#"
result=0
number=0
for file do
	number=$((number + 1))
	BATS_FILE_TMPDIR=$(mktemp -d "$RECORD_DIR/file.XXXXXX")
	export BATS_FILE_TMPDIR
	name=${file##*/}
	name=${name%.bats}
	if /bin/sh "$file"; then
		printf 'ok %s isolated %s\n' "$number" "$name"
	else
		printf 'not ok %s fails\n' "$number"
		result=1
	fi
done
exit "$result"
`
	stubPath := filepath.Join(binDir, "bats")
	require.NoError(t, os.WriteFile(stubPath, []byte(stub), 0o600))
	//nolint:gosec // The temporary Bats stub must be executable to test command lookup.
	require.NoError(t, os.Chmod(stubPath, 0o700))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, cpuCount := range []int{1, 4, 8} {
		t.Run(strconv.Itoa(cpuCount), func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(root, "tests"), 0o750))
			// Each fixture records its file directory outside the source tree.
			for _, name := range []string{"a", "b"} {
				body := "set -eu\n[ ! -e \"$BATS_FILE_TMPDIR/owner\" ]\necho " + name + " >\"$BATS_FILE_TMPDIR/owner\"\nprintf '%s' \"$BATS_FILE_TMPDIR\" >\"$RECORD_DIR/" + name + "\"\n"
				require.NoError(t, os.WriteFile(filepath.Join(root, "tests", name+".bats"), []byte(body), 0o600))
			}
			recordDir := t.TempDir()
			t.Setenv("RECORD_DIR", recordDir)
			var output []byte
			run := func(args []string) error {
				cmd := exec.CommandContext(t.Context(), "bats", args...)
				var err error
				output, err = cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("fixture Bats run: %w", err)
				}
				return nil
			}
			require.NoError(t, runSuite(root, cpuCount, nil, run), string(output))
			require.Contains(t, string(output), "1..2")
			require.Contains(t, string(output), "ok 1 isolated a")
			require.Contains(t, string(output), "ok 2 isolated b")
			jobs, err := os.ReadFile(filepath.Join(recordDir, "jobs"))
			require.NoError(t, err)
			require.Equal(t, strconv.Itoa(cpuCount), string(jobs))
			a, err := os.ReadFile(filepath.Join(recordDir, "a"))
			require.NoError(t, err)
			b, err := os.ReadFile(filepath.Join(recordDir, "b"))
			require.NoError(t, err)
			require.NotEqual(t, string(a), string(b))
			require.NotContains(t, string(a), root)
			require.NotContains(t, string(b), root)
			require.NoError(
				t,
				os.WriteFile(filepath.Join(root, "tests/fail.bats"), []byte("exit 1\n"), 0o600),
			)
			require.Error(t, runSuite(root, cpuCount, nil, run))
			require.Contains(t, string(output), "1..3")
			require.Contains(t, string(output), "not ok 3 fails")
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Len(t, entries, 1, "Bats must not write into the source tree")
		})
	}
}

func TestTaskRunsWholeSuiteAndKeepsCICheckNames(t *testing.T) {
	t.Parallel()
	task, err := os.ReadFile("../../mise.toml")
	require.NoError(t, err)
	require.Contains(t, string(task), "go run ./cmd/test-hooks")
	require.NotContains(t, string(task), `bats --recursive $usage_path`)
	require.Contains(t, string(task), `rush = "0.11.0"`)
	workflow, err := os.ReadFile("../../.github/workflows/ci.yml")
	require.NoError(t, err)
	require.Contains(t, string(workflow), `name: "test-bats (${{ matrix.os }})"`)
	require.Contains(t, string(workflow), "run: mise run test-hooks")
}
