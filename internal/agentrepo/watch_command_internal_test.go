// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMain also provides a portable GitHub CLI fixture under the `gh` executable name.
func TestMain(m *testing.M) {
	if strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") == "gh" {
		os.Exit(watchFixtureGH())
	}
	os.Exit(m.Run())
}

func watchFixtureGH() int {
	dir, err := os.Getwd()
	if err != nil {
		return 9
	}
	logRoot, err := os.OpenRoot(os.Getenv("WATCH_FIXTURE_LOG"))
	if err != nil {
		return 9
	}
	defer logRoot.Close()
	log, err := logRoot.OpenFile("requests", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 9
	}
	defer log.Close()
	if _, err = fmt.Fprintln(log, dir+" "+strings.Join(os.Args[1:], " ")); err != nil {
		return 9
	}
	for _, key := range []string{"GH_REPO", "GH_HOST", "CDPATH", "GREP_OPTIONS"} {
		if os.Getenv(key) != "" {
			return 9
		}
	}
	switch strings.Join(os.Args[1:3], " ") {
	case "api graphql":
		fmt.Fprintln(os.Stdout, "agent")
	case "pr list":
		fmt.Fprintln(os.Stdout, "7")
	case "pr checks":
		if os.Getenv("WATCH_FIXTURE_FAILURE") != "" {
			fmt.Fprintln(os.Stderr, "HTTP 401: Bad credentials")
			return 1
		}
		bucket := cmp.Or(os.Getenv("WATCH_FIXTURE_BUCKET"), "pass")
		fmt.Fprintf(os.Stdout, `[{"name":"build","workflow":"ci","bucket":%q,"link":"/1"}]`+"\n", bucket)
	default:
		return 9
	}
	return 0
}

func TestWatchCommandRepository(t *testing.T) {
	t.Setenv("PR_CHECKS_APPEAR_TIMEOUT", "0")
	t.Setenv("PR_CHECKS_INTERVAL", "0")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Chdir(root)
	git, err := exec.LookPath("git")
	require.NoError(t, err)
	sh := func(args ...string) {
		t.Helper()
		data, runErr := exec.CommandContext(t.Context(), git, args...).CombinedOutput()
		require.NoError(t, runErr, "%s", data)
	}
	write := func(name, text string) { require.NoError(t, os.WriteFile(name, []byte(text), 0o600)) }
	sh("init", "--quiet", "--initial-branch=topic")
	write("tracked.txt", "base\n")
	sh("add", "tracked.txt")
	sh("-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "--quiet", "-m", "base")
	write("tracked.txt", "edited\n")
	write("staged.txt", "staged\n")
	sh("add", "staged.txt")
	write("untracked.txt", "loose\n")
	require.NoError(t, os.Mkdir("nested", 0o700))
	bin := t.TempDir()
	exe, err := os.Executable()
	require.NoError(t, err)
	fixture, err := os.ReadFile(exe)
	require.NoError(t, err)
	name := "gh"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binRoot, err := os.OpenRoot(bin)
	require.NoError(t, err)
	defer binRoot.Close()
	require.NoError(t, binRoot.WriteFile(name, fixture, 0o700))
	logDir := t.TempDir()
	log := filepath.Join(logDir, "requests")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WATCH_FIXTURE_LOG", logDir)
	for _, key := range []string{"GH_REPO", "GH_HOST", "CDPATH", "GREP_OPTIONS"} {
		t.Setenv(key, "redirect")
	}
	before := watchGitState(t, git, root)
	t.Chdir(filepath.Join(root, "nested"))
	run := func(args ...string) (int, string, string) {
		var out, stderr bytes.Buffer
		status := Run(append([]string{"checks", "watch"}, args...), nil, &out, &stderr)
		return status, out.String(), stderr.String()
	}
	status, out, stderr := run()
	require.Equal(t, 0, status, stderr)
	require.Equal(t, "WATCHING #7\nPASS  build\nALL GREEN #7\n", out)
	requests, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(string(requests), "pr checks 7 --json name,bucket,link,workflow"))
	for line := range strings.SplitSeq(strings.TrimSpace(string(requests)), "\n") {
		require.True(t, strings.HasPrefix(line, root+" "), line)
	}
	require.Equal(t, before, watchGitState(t, git, root))
	for _, c := range []struct {
		bucket, failure, timeout, want string
		status                         int
	}{
		{"fail", "", "", "FAILED #7 with 1 failing check(s)", 1},
		{"pending", "", "0", "TIMEOUT #7", 2},
		{"pass", "1", "", "ERROR could not read checks for #7: HTTP 401", 2},
	} {
		t.Setenv("WATCH_FIXTURE_BUCKET", c.bucket)
		t.Setenv("WATCH_FIXTURE_FAILURE", c.failure)
		t.Setenv("PR_CHECKS_TIMEOUT", c.timeout)
		status, out, _ = run("7")
		require.Equal(t, c.status, status)
		require.Contains(t, out, c.want)
		require.Equal(t, before, watchGitState(t, git, root))
	}
	requests, err = os.ReadFile(log)
	require.NoError(t, err)
	t.Chdir(t.TempDir())
	status, out, stderr = run("7")
	require.Equal(t, 2, status)
	require.Empty(t, out)
	require.Contains(t, stderr, "not a git repository")
	after, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Equal(t, string(requests), string(after))
}

func watchGitState(t *testing.T, git, root string) string {
	t.Helper()
	var state strings.Builder
	for _, cmd := range []*exec.Cmd{
		exec.CommandContext(t.Context(), git, "status", "--porcelain=v1"),
		exec.CommandContext(t.Context(), git, "diff", "HEAD"),
		exec.CommandContext(t.Context(), git, "for-each-ref"),
	} {
		cmd.Dir = root
		out, err := cmd.Output()
		require.NoError(t, err)
		state.Write(out)
	}
	loose, err := os.ReadFile(filepath.Join(root, "untracked.txt"))
	require.NoError(t, err)
	return state.String() + string(loose)
}
