// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package bats

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJobs(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, override string
		cpus, want     int
		bad            bool
	}{
		{"single CPU", "", 1, 1, false},
		{"CPU default", "", 4, 4, false},
		{"CPU cap", "", 32, 8, false},
		{"serial override", "1", 32, 1, false},
		{"parallel override", "4", 32, 4, false},
		{"override cap", "99", 32, 8, false},
		{"zero", "0", 4, 0, true},
		{"negative", "-1", 4, 0, true},
		{"invalid", "bad", 4, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := jobs(tt.override, tt.cpus)
			if got != tt.want || (err != nil) != tt.bad {
				t.Fatalf("jobs = %d, %v; want %d, error %v", got, err, tt.want, tt.bad)
			}
		})
	}
}

func TestPartition(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name            string
		files           []string
		serial          map[string]string
		parallel, alone []string
	}{
		{name: "empty"},
		{name: "all parallel", files: []string{"a.bats", "b.bats"}, parallel: []string{"a.bats", "b.bats"}},
		{name: "mixed", files: []string{"a.bats", "b.bats"}, serial: map[string]string{"a.bats": "shared port"}, parallel: []string{"b.bats"}, alone: []string{"a.bats"}},
		{name: "all serial", files: []string{"a.bats"}, serial: map[string]string{"a.bats": "shared port"}, alone: []string{"a.bats"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parallel, alone := partition(tt.files, tt.serial)
			require.Equal(t, tt.parallel, parallel)
			require.Equal(t, tt.alone, alone)
		})
	}
}

func TestDiscovery(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"z.bats", "nested/a.bats", "ignore.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct {
		name, path string
		want       []string
		bad        bool
	}{
		{"recursive sorted", root, []string{filepath.Join(nested, "a.bats"), filepath.Join(root, "z.bats")}, false},
		{"single file", filepath.Join(root, "z.bats"), []string{filepath.Join(root, "z.bats")}, false},
		{"missing", filepath.Join(root, "missing"), nil, true},
		{"empty directory", t.TempDir(), nil, true},
		{"other extension", filepath.Join(root, "ignore.txt"), nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := discover(tt.path)
			require.Equal(t, tt.want, got)
			if (err != nil) != tt.bad {
				t.Fatalf("discover = %v, %v", got, err)
			}
		})
	}
}

func TestRunRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, path, override string }{
		{"missing path", filepath.Join(t.TempDir(), "missing"), "1"},
		{"invalid jobs", t.TempDir(), "bad"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, diagnostic bytes.Buffer
			if status := Run(
				t.Context(),
				tt.path,
				tt.override,
				&out,
				&diagnostic,
			); status == 0 ||
				diagnostic.Len() == 0 {
				t.Fatalf("status %d, diagnostic %q", status, diagnostic.String())
			}
		})
	}
}

func TestStatusAndOrder(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		jobs    int
		results []int
		want    int
	}{
		{"success", 4, []int{0, 0}, 0},
		{"parallel fails", 4, []int{7, 0}, 7},
		{"serial fails", 1, []int{0, 9}, 9},
		{"both fail", 8, []int{7, 9}, 7},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls [][]string
			status := execute([]string{"p.bats"}, []string{"s.bats"}, tt.jobs, func(args []string) int {
				calls = append(calls, args)
				return tt.results[len(calls)-1]
			})
			parallel := []string{"p.bats"}
			if tt.jobs > 1 {
				parallel = slices.Concat([]string{
					"--jobs", "4",
					"--parallel-binary-name", "rush",
					"--no-parallelize-within-files",
				}, parallel)
			}
			if tt.jobs == 8 {
				parallel[1] = "8"
			}
			want := [][]string{parallel, {"s.bats"}}
			require.Equal(t, tt.want, status)
			require.Equal(t, want, calls)
		})
	}
}

func TestExecuteEmptyGroups(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name             string
		parallel, serial []string
		calls            int
	}{
		{name: "parallel only", parallel: []string{"p.bats"}, calls: 1},
		{name: "serial only", serial: []string{"s.bats"}, calls: 1},
		{name: "neither"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			status := execute(tt.parallel, tt.serial, 1, func([]string) int { calls++; return 0 })
			require.Zero(t, status)
			require.Equal(t, tt.calls, calls)
		})
	}
}

func TestMain(m *testing.M) {
	code := os.Getenv("BATS_RUNNER_TEST_EXIT")
	if code == "" {
		os.Exit(m.Run())
	}
	if len(os.Args) > 1 && os.Args[1] == "install" {
		os.Exit(fakeBuild())
	}
	os.Exit(fakeBats(code))
}

func fakeBuild() int {
	name := "agentrepo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	output := filepath.Join(os.Getenv("GOBIN"), name)
	_, _ = fmt.Fprintln(os.Stdout, "helper:", output)
	if os.Getenv("BATS_RUNNER_BUILD_FAIL") != "" {
		return 6
	}
	executable, err := os.Executable()
	if err != nil {
		return 98
	}
	if err = os.Link(executable, output); err != nil {
		return 97
	}
	return 0
}

func fakeBats(code string) int {
	if os.Getenv("BATS_RUNNER_CHECK_HELPER") != "" {
		if _, err := exec.LookPath("agentrepo"); err != nil {
			return 96
		}
	}
	status, err := strconv.Atoi(code)
	if err != nil {
		return 99
	}
	return status
}

func installTool(t *testing.T, executable, bin, name string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	require.NoError(t, os.Link(executable, filepath.Join(bin, name)))
}

func TestRunBats(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, tt := range []struct {
		name, override, code string
		installed            bool
		want                 int
	}{
		{"serial success", "1", "0", true, 0},
		{"parallel success", "4", "0", true, 0},
		{"Bats status", "1", "7", true, 7},
		{"Bats unavailable", "1", "0", false, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "suite.bats"), nil, 0o600))
			bin := t.TempDir()
			installTool(t, executable, bin, "go")
			if tt.installed {
				installTool(t, executable, bin, "bats")
			}
			t.Setenv("PATH", bin)
			t.Setenv("BATS_RUNNER_TEST_EXIT", tt.code)
			var out, diagnostic bytes.Buffer
			status := Run(t.Context(), root, tt.override, &out, &diagnostic)
			if status != tt.want || !strings.Contains(out.String(), tt.override+" jobs") {
				t.Fatalf("status %d, out %q, diagnostic %q", status, out.String(), diagnostic.String())
			}
			if tt.want != 0 && diagnostic.Len() == 0 {
				t.Fatal("missing failure diagnostic")
			}
		})
	}
}

func TestUnreadableDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	info, err := os.Stat(root)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(root, 0))
	t.Cleanup(func() {
		require.NoError(t, os.Chmod(root, info.Mode().Perm()))
	})
	if _, readErr := os.ReadDir(root); readErr == nil {
		t.Skip("this account can read directories without permission bits")
	}
	var out, diagnostic bytes.Buffer
	if status := Run(
		t.Context(),
		root,
		"1",
		&out,
		&diagnostic,
	); status == 0 ||
		!strings.Contains(diagnostic.String(), "discovering") {
		t.Fatalf("status %d, diagnostic %q", status, diagnostic.String())
	}
}

func TestRunBuildsIsolatedHelper(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, fail := range []bool{false, true} {
		t.Run(strconv.FormatBool(fail), func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "suite.bats"), nil, 0o600))
			bin := t.TempDir()
			installTool(t, executable, bin, "go")
			installTool(t, executable, bin, "bats")
			t.Setenv("PATH", bin)
			t.Setenv("BATS_RUNNER_TEST_EXIT", "0")
			t.Setenv("BATS_RUNNER_CHECK_HELPER", "1")
			if fail {
				t.Setenv("BATS_RUNNER_BUILD_FAIL", "1")
			}
			var out, diagnostic bytes.Buffer
			status := Run(t.Context(), root, "4", &out, &diagnostic)
			if fail {
				require.NotZero(t, status)
			} else {
				require.Zero(t, status, diagnostic.String())
			}
			require.Equal(t, 1, strings.Count(out.String(), "helper: "))
			_, built, found := strings.Cut(out.String(), "helper: ")
			require.True(t, found)
			path, _, _ := strings.Cut(built, "\n")
			require.True(t, filepath.IsAbs(path))
			_, statErr := os.Stat(filepath.Dir(path))
			require.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestRunTempFailure(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "suite.bats"), nil, 0o600))
	t.Setenv("TMPDIR", filepath.Join(root, "missing"))
	t.Setenv("TMP", filepath.Join(root, "missing"))
	t.Setenv("TEMP", filepath.Join(root, "missing"))
	var out, diagnostic bytes.Buffer
	require.NotZero(t, Run(t.Context(), root, "1", &out, &diagnostic))
	require.Contains(t, diagnostic.String(), "test-hooks:")
}
