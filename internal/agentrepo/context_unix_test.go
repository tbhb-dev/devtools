//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

type discoveryResult struct {
	status      int
	out, stderr string
}

func discoveryShell(t *testing.T, lib string, args ...string) discoveryResult {
	t.Helper()
	//nolint:gosec // Fixed test script and fixture arguments.
	process := exec.CommandContext(
		t.Context(),
		"bash",
		append([]string{"-c", `. "$1"; shift; "$@"`, "_", lib}, args...)...,
	)
	var out, stderr bytes.Buffer
	process.Stdout, process.Stderr = &out, &stderr
	err := process.Run()
	status := 0
	if err != nil {
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit)
		status = exit.ExitCode()
	}
	return discoveryResult{status, out.String(), stderr.String()}
}

func discoveryGo(kind string, args ...string) discoveryResult {
	var out, stderr bytes.Buffer
	status := agentrepo.Run(append([]string{"context", kind}, args...), nil, &out, &stderr)
	return discoveryResult{status, out.String(), stderr.String()}
}

func discoverySetup(t *testing.T) string {
	t.Helper()
	lib, err := filepath.Abs("../../packages/agents/source/lib/lib.sh")
	require.NoError(t, err)
	t.Chdir(t.TempDir())
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "CLAUDE_CODE_SESSION_ID", "CLAUDECODE", "CODEX_THREAD_ID", "DEVTOOLS_HARNESS"} {
		t.Setenv(key, "")
	}
	// Git treats an empty GIT_DIR as an explicit invalid directory.
	require.NoError(t, os.Unsetenv("GIT_DIR"))
	require.NoError(t, os.Unsetenv("GIT_WORK_TREE"))
	require.NoError(t, os.Unsetenv("GIT_INDEX_FILE"))
	require.NoError(t, os.Unsetenv("GIT_COMMON_DIR"))
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for key, value := range map[string]string{"_DEVTOOLS_AGENTREPO_BIN": "", "_DEVTOOLS_AGENTREPO_CONFIG": "false", "_DEVTOOLS_AGENTREPO_VERDICT": "false", "_DEVTOOLS_AGENTREPO_CONTEXT": "false"} {
		t.Setenv(key, value)
	}
	discoveryGit(t, "init", "--quiet", "--initial-branch=main")
	discoveryGit(t, "config", "user.name", "Tester")
	discoveryGit(t, "config", "user.email", "tester@example.com")
	return lib
}

func discoveryGit(t *testing.T, args ...string) {
	t.Helper()
	process := exec.CommandContext(t.Context(), "git", args...)
	data, err := process.CombinedOutput()
	require.NoError(t, err, "%s", data)
}

//nolint:paralleltest // Mutates process cwd and environment.
func TestDiscoveryCorpus(t *testing.T) {
	lib := discoverySetup(t)
	cases := []struct {
		kind, function string
		args           []string
	}{
		{"root", "dt_root", nil},
		{"git-dir", "dt_git_dir", nil},
		{"repo-name", "dt_repo_name", nil},
		{"state-dir", "dt_state_dir", nil},
		{"head", "dt_head", nil},
		{"agent-context", "dt_agent_context", nil},
		{"path", "dt_path", []string{"~/space here"}},
		{"path", "dt_path", []string{"literal/~"}},
		{"digest", "dt_digest", []string{"draft"}},
	}
	require.NoError(t, os.WriteFile("draft", []byte("recorded corpus\n"), 0o600))
	compare := func(t *testing.T) {
		t.Helper()
		for _, tc := range cases {
			t.Run(tc.kind+strings.Join(tc.args, ""), func(t *testing.T) {
				want := discoveryShell(t, lib, append([]string{tc.function}, tc.args...)...)
				require.Equal(t, want, discoveryGo(tc.kind, tc.args...))
			})
		}
	}
	t.Run("unborn", compare)
	discoveryGit(t, "commit", "--quiet", "--allow-empty", "-m", "fixture")
	t.Run("committed", compare)
	for _, marker := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply"} {
		t.Run(marker, func(t *testing.T) {
			file := filepath.Join(".git", marker)
			require.NoError(t, os.WriteFile(file, []byte("marker"), 0o600))
			t.Cleanup(func() { require.NoError(t, os.Remove(file)) })
			for _, action := range []string{"", "commit", "pr"} {
				require.Equal(
					t,
					discoveryShell(t, lib, "dt_agent_context", action),
					discoveryGo("agent-context", action),
				)
			}
		})
	}
	discoveryGit(t, "checkout", "--quiet", "--detach")
	t.Run("detached", compare)
	discoveryGit(t, "checkout", "--quiet", "main")
	linked := filepath.Join(t.TempDir(), "linked")
	discoveryGit(t, "worktree", "add", "--quiet", "-b", "linked", linked)
	t.Chdir(linked)
	require.NoError(t, os.WriteFile("draft", []byte("recorded corpus\n"), 0o600))
	t.Run("linked", compare)
}

func TestDiscoveryHarnessMatrix(t *testing.T) {
	lib := discoverySetup(t)
	keys := []string{"DEVTOOLS_HARNESS", "CLAUDE_CODE_SESSION_ID", "CLAUDECODE", "CODEX_THREAD_ID"}
	for bits := range 16 {
		t.Run(string(rune('A'+bits)), func(t *testing.T) {
			for index, key := range keys {
				value := ""
				if bits&(1<<index) != 0 {
					value = "set"
				}
				t.Setenv(key, value)
			}
			require.Equal(t, discoveryShell(t, lib, "dt_harness"), discoveryGo("harness"))
			require.Equal(t, discoveryShell(t, lib, "dt_session_id"), discoveryGo("session-id"))
		})
	}
	for _, value := range []string{"claude", "codex", "unknown", "custom"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("DEVTOOLS_HARNESS", value)
			require.Equal(t, discoveryShell(t, lib, "dt_harness"), discoveryGo("harness"))
		})
	}
	for _, dir := range []string{"/x/.claude/skills/commit/scripts", "/x/.agents/skills/commit/scripts", "/x/packages/agents/claude/a", "/x/packages/agents/codex/a", "/unknown"} {
		process := exec.CommandContext(t.Context(), "bash", "-c", `. "$1"; DT_LIB_DIR=$2; dt_harness`, "_", lib, dir)
		data, err := process.Output()
		require.NoError(t, err)
		require.Equal(t, string(data), discoveryGo("harness", "--lib-dir", dir).out)
	}
}

//nolint:paralleltest // Mutates process cwd and environment.
func TestDiscoveryProducerFailures(t *testing.T) {
	lib := discoverySetup(t)
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, output := range []string{"", "abc  draft\n"} {
		t.Run(output, func(t *testing.T) {
			require.NoError(
				t,
				discoveryExecutable(
					filepath.Join(bin, "sha256sum"),
					[]byte("#!/bin/sh\nprintf '"+output+"'\nexit 9\n"),
				),
			)
			require.Equal(t, discoveryResult{status: 9}, discoveryShell(t, lib, "dt_digest", "draft"))
			require.Equal(t, discoveryResult{status: 9}, discoveryGo("digest", "draft"))
		})
	}
	for _, message := range []string{"not a repository", "rev-parse failed", "unreadable Git directory"} {
		t.Run(message, func(t *testing.T) {
			require.NoError(
				t,
				discoveryExecutable(
					filepath.Join(bin, "git"),
					[]byte("#!/bin/sh\necho partial\necho '"+message+"' >&2\nexit 9\n"),
				),
			)
			for _, kind := range []string{"root", "git-dir", "repo-name", "state-dir", "head", "agent-context", "ready"} {
				require.Equal(t, discoveryResult{9, "", message + "\n"}, discoveryGo(kind))
			}
		})
	}
}

//nolint:paralleltest // Mutates process cwd and environment.
func TestDiscoveryReadinessCorpus(t *testing.T) {
	lib := discoverySetup(t)
	ready := filepath.Join(filepath.Dir(lib), "worktree-ready.sh")
	bin := t.TempDir()
	// Only these commands can be called. Optional tools are absent from PATH.
	for _, name := range []string{"bash", "git", "dirname"} {
		target, err := exec.LookPath(name)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(target, filepath.Join(bin, name)))
	}
	t.Setenv("PATH", bin)
	states := []struct{ name, config, mise, vale string }{
		{name: "absent tools", config: "mise.toml"},
		{name: "absent config", mise: "exit 9", vale: "exit 9"},
		{name: "untrusted", config: "mise.toml", mise: "echo 'fixture: untrusted'"},
		{name: "trust query failure", config: ".mise.toml", mise: "echo broken >&2; exit 9"},
		{name: "trusted", config: "mise.toml", mise: "echo 'fixture: trusted'"},
		{name: "vale absent", config: ".vale.ini"},
		{name: "vale success", config: ".vale.ini", vale: "exit 0"},
		{name: "vale findings", config: ".vale.ini", vale: "echo finding; exit 1"},
		{
			name:   "vale missing styles",
			config: ".vale.ini",
			vale:   "echo \"Style 'Google' isn't on the StylesPath\" >&2; exit 2",
		},
		{name: "vale missing path", config: ".vale.ini", vale: "echo \"StylesPath 'x' doesn't exist\"; exit 2"},
		{name: "vale other failure", config: ".vale.ini", vale: "echo broken >&2; exit 3"},
	}
	for _, tc := range states {
		t.Run(tc.name, func(t *testing.T) {
			if tc.config != "" {
				require.NoError(t, os.WriteFile(tc.config, nil, 0o600))
				t.Cleanup(func() { require.NoError(t, os.Remove(tc.config)) })
			}
			discoveryReadinessTools(t, bin, tc.mise, tc.vale)
			before, err := os.ReadDir(".")
			require.NoError(t, err)
			// The shell helper sources the frozen fallback before running readiness.
			want := discoveryShell(t, lib, "bash", ready)
			require.Equal(t, want, discoveryGo("ready"))
			after, err := os.ReadDir(".")
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func discoveryExecutable(path string, data []byte) error {
	//nolint:gosec // Executable fixture scripts run in a temporary directory.
	if err := os.WriteFile(path, data, 0o700); err != nil {
		return fmt.Errorf("write executable fixture: %w", err)
	}
	return nil
}

func discoveryReadinessTools(t *testing.T, bin, mise, vale string) {
	t.Helper()

	for name, body := range map[string]string{"mise": mise, "vale": vale} {
		if body != "" {
			path := filepath.Join(bin, name)
			// Any trust change, download or unexpected command is a test failure.
			var guard string
			if name == "mise" {
				guard = "[ \"$*\" = \"-C $PWD trust --show\" ] || { echo unexpected >&2; exit 99; }\n"
			} else {
				guard = "[ \"$*\" = '--ext=.md --output=JSON' ] || exit 99\n"
			}
			require.NoError(t, discoveryExecutable(path, []byte("#!/bin/sh\n"+guard+body+"\n")))
			t.Cleanup(func() { require.NoError(t, os.Remove(path)) })
		}
	}
}

func TestDiscoveryHeadVerificationFailure(t *testing.T) {
	discoverySetup(t)
	discoveryGit(t, "commit", "--quiet", "--allow-empty", "-m", "fixture")
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	t.Setenv("REAL_CONTEXT_GIT", realGit)
	bin := t.TempDir()
	require.NoError(t, discoveryExecutable(filepath.Join(bin, "git"), []byte(`#!/bin/sh
case "$*" in
*'rev-parse --verify --quiet HEAD') echo complete-looking-head; echo 'verification failed' >&2; exit 1 ;;
esac
exec "$REAL_CONTEXT_GIT" "$@"
`)))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	require.Equal(t, discoveryResult{1, "", "verification failed\n"}, discoveryGo("head"))
}

//nolint:paralleltest // Mutates process cwd and environment.
func TestDiscoveryStateCreation(t *testing.T) {
	discoverySetup(t)
	result := discoveryGo("state-dir")
	require.Equal(t, 0, result.status)
	require.Empty(t, result.stderr)
	info, err := os.Stat(strings.TrimSuffix(result.out, "\n"))
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestDiscoveryDigestFallback(t *testing.T) {
	lib := discoverySetup(t)
	bin := t.TempDir()
	for _, name := range []string{"bash", "dirname"} {
		target, err := exec.LookPath(name)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(target, filepath.Join(bin, name)))
	}
	t.Setenv("PATH", bin)
	for _, status := range []string{"0", "9"} {
		require.NoError(
			t,
			discoveryExecutable(
				filepath.Join(bin, "shasum"),
				[]byte("#!/bin/sh\n[ \"$*\" = '-a 256 draft' ] || exit 99\nprintf 'abc  draft\\n'\nexit "+status+"\n"),
			),
		)
		require.Equal(t, discoveryShell(t, lib, "dt_digest", "draft"), discoveryGo("digest", "draft"))
	}
}
