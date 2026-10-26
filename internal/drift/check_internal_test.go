// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package drift

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	data, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", data)
	return string(data)
}

func write(t *testing.T, dir, path, body string) {
	t.Helper()
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer root.Close()
	require.NoError(t, root.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, root.WriteFile(path, []byte(body), 0o600))
}

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.name", "Fixture")
	git(t, dir, "config", "user.email", "fixture@example.com")
	for _, path := range []string{"go.mod", "go.sum", "vendor/modules.txt", "vendor/tracked", "vendir.yml", "vendir.lock.yml", ".config/mise/conf.d/tracked.toml", ".devtools/scripts/tracked", ".devtools/config/tracked"} {
		write(t, dir, path, "tracked\n")
	}
	write(t, dir, ".gitignore", "ignored\n")
	git(
		t,
		dir,
		"add",
		"--",
		"go.mod",
		"go.sum",
		"vendor",
		"vendir.yml",
		"vendir.lock.yml",
		".config",
		".devtools",
		".gitignore",
	)
	git(t, dir, "-c", "commit.gpgSign=false", "-c", "core.hooksPath=", "commit", "-qm", "fixture")
	git(t, dir, "remote", "add", "origin", "https://example.com/fixture.git")
	return dir
}

func runFixture(t *testing.T, dir string, vendir bool) (int, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	run := func(ctx context.Context, name string, args, env []string, stdout, diagnostics io.Writer) int {
		if name != "git" {
			return 0
		}
		return execute(ctx, dir, name, args, env, stdout, diagnostics)
	}
	code := check(t.Context(), vendir, run, &out, &stderr)
	return code, out.String()
}

func TestManifestAndRegeneration(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"absent", "untracked", "tracked"} {
		for _, failure := range []string{"", "tidy", "vendor"} {
			t.Run(state+failure, func(t *testing.T) { t.Parallel(); manifestScenario(t, state, failure) })
		}
	}
}

func manifestScenario(t *testing.T, state, failure string) {
	t.Helper()
	dir := fixture(t)
	if state != "tracked" {
		git(t, dir, "rm", "-f", "--", "vendor/modules.txt")
	}
	if state == "untracked" {
		write(t, dir, "vendor/modules.txt", "untracked\n")
	}
	var calls []string
	var out, stderr bytes.Buffer
	run := func(ctx context.Context, name string, args, env []string, stdout, diagnostics io.Writer) int {
		if name == "git" {
			return execute(ctx, dir, name, args, env, stdout, diagnostics)
		}
		calls = append(calls, strings.Join(args, " "))
		if args[1] == failure {
			return 23
		}
		return 0
	}
	code := check(t.Context(), false, run, &out, &stderr)
	if state != "tracked" {
		require.Zero(t, code)
		require.Empty(t, calls)
		require.Equal(t, "vendor-check: skipping module without tracked vendor/modules.txt\n", out.String())
		return
	}
	want := []string{"mod tidy"}
	if failure != "tidy" {
		want = append(want, "mod vendor")
	}
	require.Equal(t, want, calls)
	expected := 0
	if failure != "" {
		expected = 23
	}
	require.Equal(t, expected, code)
}

func TestPayloadAndIndex(t *testing.T) {
	t.Parallel()
	for _, vendir := range []bool{false, true} {
		paths := []string{"go.mod", "go.sum", "vendor/modules.txt", "vendor/tracked"}
		if vendir {
			paths = []string{
				"vendir.yml",
				"vendir.lock.yml",
				".config/mise/conf.d/tracked.toml",
				".devtools/scripts/tracked",
				".devtools/config/tracked",
			}
		}
		for _, path := range paths {
			t.Run(path, func(t *testing.T) {
				t.Parallel()
				dir := fixture(t)
				before := snapshot(t, dir)
				write(t, dir, path, "modified\n")
				code, out := runFixture(t, dir, vendir)
				require.Equal(t, 1, code)
				require.Contains(t, out, path)
				require.Equal(t, before, snapshot(t, dir))
				git(t, dir, "add", "--", path)
				staged := snapshot(t, dir)
				code, _ = runFixture(t, dir, vendir)
				require.Zero(t, code)
				require.Equal(t, staged, snapshot(t, dir))
				require.NoError(t, os.Remove(filepath.Join(dir, path)))
				code, out = runFixture(t, dir, vendir)
				require.Equal(t, 1, code)
				require.Contains(t, out, path)
				require.Equal(t, staged, snapshot(t, dir))
			})
		}
	}
}

func snapshot(t *testing.T, dir string) string {
	t.Helper()
	return git(t, dir, "ls-files", "--stage") + git(t, dir, "rev-parse", "HEAD") + git(t, dir, "remote", "-v")
}

func TestUntrackedAndIgnores(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"vendor/new", ".devtools/scripts/new", ".devtools/config/new", ".config/mise/conf.d/new"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			dir := fixture(t)
			vendir := !strings.HasPrefix(path, "vendor/")
			global := filepath.Join(t.TempDir(), "ignore")
			require.NoError(t, os.WriteFile(global, []byte("new\n"), 0o600))
			git(t, dir, "config", "core.excludesFile", global)
			write(t, dir, ".git/info/exclude", "local-hidden\n")
			write(t, dir, path, "new\n")
			local := filepath.Join(filepath.Dir(path), "local-hidden")
			write(t, dir, local, "local\n")
			write(t, dir, filepath.Join(filepath.Dir(path), "ignored"), "ignored\n")
			code, out := runFixture(t, dir, vendir)
			require.Equal(t, 1, code)
			require.Contains(t, out, path)
			require.Contains(t, out, filepath.ToSlash(local))
			require.NotContains(t, out, "ignored")
			require.NoError(t, os.Remove(filepath.Join(dir, path)))
			require.NoError(t, os.Remove(filepath.Join(dir, local)))
			code, _ = runFixture(t, dir, vendir)
			require.Zero(t, code)
		})
	}
}

func TestGitFailures(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"manifest", "diff", "listing"} {
		for _, vendir := range []bool{false, true} {
			t.Run(operation, func(t *testing.T) {
				t.Parallel()
				var out, stderr bytes.Buffer
				run := failingGit(t, operation)
				code := check(t.Context(), vendir, run, &out, &stderr)
				if operation == "manifest" && vendir {
					require.Zero(t, code)
				} else {
					require.NotZero(t, code)
					require.Contains(t, stderr.String(), "git failed")
				}
			})
		}
	}
}

func TestVendirEnvironment(t *testing.T) {
	t.Setenv("GIT_DIR", "caller-GIT_DIR")
	keys := []string{"GIT_DIR", "GIT_COMMON_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY"}
	for _, key := range keys {
		t.Setenv(key, "caller-"+key)
	}
	var names []string
	run := func(_ context.Context, name string, _, env []string, _, _ io.Writer) int {
		names = append(names, name)
		for _, key := range keys {
			found := false
			for _, entry := range env {
				if strings.HasPrefix(entry, key+"=") {
					found = true
				}
			}
			require.Equal(t, name == "git", found, key)
		}
		return 0
	}
	var out, stderr bytes.Buffer
	require.Zero(t, check(t.Context(), true, run, &out, &stderr))
	require.Equal(t, []string{"vendir", "git", "git"}, names)
}

func TestAlternateIndex(t *testing.T) {
	dir := fixture(t)
	index := filepath.Join(t.TempDir(), "index")
	original, err := os.ReadFile(filepath.Join(dir, ".git", "index"))
	require.NoError(t, err)
	write(t, filepath.Dir(index), "index", string(original))
	t.Setenv("GIT_INDEX_FILE", index)
	write(t, dir, ".devtools/scripts/tracked", "alternate staged\n")
	git(t, dir, "add", "--", ".devtools/scripts/tracked")
	before, err := os.ReadFile(index)
	require.NoError(t, err)
	code, _ := runFixture(t, dir, true)
	require.Zero(t, code)
	after, err := os.ReadFile(index)
	require.NoError(t, err)
	require.Equal(t, before, after)
	normal, err := os.ReadFile(filepath.Join(dir, ".git", "index"))
	require.NoError(t, err)
	require.Equal(t, original, normal)
}

func failingGit(t *testing.T, operation string) runner {
	t.Helper()
	return func(_ context.Context, name string, args, _ []string, stdout, diagnostics io.Writer) int {
		if name != "git" {
			return 0
		}
		isManifest := args[0] == "ls-files" && len(args) == 3
		fail := (operation == "manifest" && isManifest) || (operation == "diff" && args[0] == "diff") ||
			(operation == "listing" && args[0] == "ls-files" && !isManifest)
		if fail {
			_, err := io.WriteString(stdout, "partial\n")
			require.NoError(t, err)
			_, err = io.WriteString(diagnostics, "git failed\n")
			require.NoError(t, err)
			return 42
		}
		if isManifest {
			_, err := io.WriteString(stdout, "vendor/modules.txt\n")
			require.NoError(t, err)
		}
		return 0
	}
}

func TestExecuteFailures(t *testing.T) {
	t.Parallel()
	var out, stderr bytes.Buffer
	require.NotZero(t, execute(t.Context(), "", "git", []string{"unknown-drift-test-command"}, nil, &out, &stderr))
	require.NotEmpty(t, stderr.String())
	require.Equal(t, commandNotFound, execute(t.Context(), "", "missing-drift-test-command", nil, nil, &out, &stderr))
	require.Equal(
		t,
		commandNotExecutable,
		execute(t.Context(), filepath.Join(t.TempDir(), "missing"), "git", nil, nil, &out, &stderr),
	)
}

func TestCheckedScopes(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"go.mod", "go.sum"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			dir := fixture(t)
			git(t, dir, "rm", "-f", "--", path)
			write(t, dir, path, "new\n")
			code, out := runFixture(t, dir, false)
			require.Equal(t, 1, code)
			require.Contains(t, out, path)
		})
	}
	t.Run("vendir manifest scan scope", func(t *testing.T) {
		t.Parallel()
		dir := fixture(t)
		git(t, dir, "rm", "-f", "--", "vendir.yml", "vendir.lock.yml")
		write(t, dir, "vendir.yml", "new\n")
		write(t, dir, "vendir.lock.yml", "new\n")
		write(t, dir, "vendor/outside-vendir", "new\n")
		code, _ := runFixture(t, dir, true)
		require.Zero(t, code)
	})
	t.Run("Go payload scan scope", func(t *testing.T) {
		t.Parallel()
		dir := fixture(t)
		write(t, dir, ".devtools/outside-go", "new\n")
		code, _ := runFixture(t, dir, false)
		require.Zero(t, code)
	})
}
