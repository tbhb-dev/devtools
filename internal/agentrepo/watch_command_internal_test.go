// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
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
		fmt.Fprintln(os.Stdout, `[{"name":"build","workflow":"ci","bucket":"pass","link":"/1"}]`)
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
	init := exec.CommandContext(t.Context(), git, "init", "--quiet", "--initial-branch=topic")
	data, err := init.CombinedOutput()
	require.NoError(t, err, "%s", data)
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
	before := watchGitState(t, git)
	t.Chdir(filepath.Join(root, "nested"))
	var out, stderr bytes.Buffer
	status := Run([]string{"checks", "watch"}, nil, &out, &stderr)
	require.Equal(t, 0, status, stderr.String())
	require.Equal(t, "WATCHING #7\nPASS  build\nALL GREEN #7\n", out.String())
	requests, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(string(requests), "pr checks 7 --json name,bucket,link,workflow"))
	for line := range strings.SplitSeq(strings.TrimSpace(string(requests)), "\n") {
		require.True(t, strings.HasPrefix(line, root+" "), line)
	}
	t.Setenv("WATCH_FIXTURE_FAILURE", "1")
	out.Reset()
	stderr.Reset()
	require.Equal(t, 2, Run([]string{"checks", "watch", "7"}, nil, &out, &stderr))
	require.Contains(t, out.String(), "ERROR could not read checks for #7: HTTP 401")
	t.Chdir(root)
	require.Equal(t, before, watchGitState(t, git))
	t.Chdir(t.TempDir())
	out.Reset()
	stderr.Reset()
	require.Equal(t, 2, Run([]string{"checks", "watch", "7"}, nil, &out, &stderr))
	require.Empty(t, out.String())
	require.Contains(t, stderr.String(), "not a git repository")
}

func watchGitState(t *testing.T, git string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), git, "status", "--porcelain=v1")
	status, err := cmd.Output()
	require.NoError(t, err)
	cmd = exec.CommandContext(t.Context(), git, "for-each-ref")
	refs, err := cmd.Output()
	require.NoError(t, err)
	return string(status) + string(refs)
}

func TestWatchProbeOffline(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Chdir(t.TempDir())
	var out, stderr bytes.Buffer
	require.Equal(t, 0, Run([]string{"checks", "watch", "--help"}, nil, &out, &stderr))
	require.Contains(t, out.String(), "agentrepo checks watch [pull-request-number]")
	require.Empty(t, stderr.String())
}
