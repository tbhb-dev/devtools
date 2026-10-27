//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

// cspell:words CDPATH GPGSIGN

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiagnosisShellParityAndReadOnly(t *testing.T) {
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	legacy, pathErr := filepath.Abs("../../packages/agents/source/skills/fix-pr/shared/scripts/diagnose-legacy.sh")
	require.NoError(t, pathErr)
	for _, scenario := range []string{"passing", "trailing space", "pending", "failures", "dirty", "branch", "commit", "both", "detached", "empty", "unreadable"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			checkDiagnosisShellParity(t, scenario, legacy)
		})
	}
}

func checkDiagnosisShellParity(t *testing.T, scenario, legacy string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	if scenario == "trailing space" {
		root = filepath.Join(root, "repository ")
		require.NoError(t, os.Mkdir(root, 0o700))
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = root
		cmd.Env = append(
			os.Environ(),
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_AUTHOR_NAME=Fixture",
			"GIT_AUTHOR_EMAIL=fixture@example.com",
			"GIT_COMMITTER_NAME=Fixture",
			"GIT_COMMITTER_EMAIL=fixture@example.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
		return strings.TrimSpace(string(out))
	}
	git("init", "-b", "topic")
	git("-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "fixture")
	head := git("rev-parse", "HEAD")
	prHead := head
	prBranch := "topic"
	switch scenario {
	case "dirty":
		require.NoError(t, os.WriteFile(filepath.Join(root, "untracked"), []byte("unchanged"), 0o600))
	case "branch":
		prBranch = "other"
	case "commit":
		prHead = strings.Repeat("a", 40)
	case "both":
		prBranch = "other"
		prHead = strings.Repeat("a", 40)
	case "detached":
		git("checkout", "--detach")
	}
	bin := t.TempDir()
	calls := filepath.Join(bin, "calls")
	prJSON := fmt.Sprintf(
		`{"number":794,"title":"Example","headRefName":%q,"headRefOid":%q,"url":"https://github.com/example/repo/pull/794"}`,
		prBranch,
		prHead,
	)
	checkJSON := `[{"name":"test","state":"SUCCESS","link":"https://example.com/check"}]`
	failed := ""
	checkExit := 0
	var logs strings.Builder
	if scenario == "pending" {
		checkJSON = strings.ReplaceAll(checkJSON, "SUCCESS", "PENDING")
	}
	if scenario == "failures" {
		checkJSON = `[{"name":"shell","state":"FAILURE","link":"https://github.com/example/repo/actions/runs/123/job/1"},{"name":"test","state":"CANCELLED","link":"https://github.com/example/repo/actions/runs/123/job/2"},{"name":"unknown","state":"TIMED_OUT","link":"https://example.com/external"}]`
		failed = "shell\thttps://github.com/example/repo/actions/runs/123/job/1\ntest\thttps://github.com/example/repo/actions/runs/123/job/2\nunknown\thttps://example.com/external\n"
		checkExit = 1
		for i := 1; i <= 100; i++ {
			fmt.Fprintf(&logs, "log %d\n", i)
		}
	}
	if scenario == "empty" {
		checkJSON = "[]"
	}
	if scenario == "unreadable" {
		checkJSON = ""
		checkExit = 2
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	script := fmt.Sprintf(`#!/usr/bin/env bash
printf '%%s\n' "$*" >> %s
case "$*" in
	'auth status') exit 0 ;;
	'pr list '*)
	if [[ "$*" == *--jq* ]]; then printf '794\n'; else printf '[{"number":794}]\n'; fi ;;
	'pr view '*--template*) printf '%%s\n' %s ;;
	'pr view '*--jq*headRefOid*) printf '%%s\n' %s ;;
	'pr view '*--jq*headRefName*) printf '%%s\n' %s ;;
	'pr view '*) printf '%%s\n' %s ;;
	'pr checks '*--json*)
	if [[ "$*" == *--jq* ]]; then printf '%%s' %s; else printf '%%s' %s; fi
	exit %d ;;
	'pr checks '*) printf 'test\tpass\thttps://example.com/check\n' ;;
	'run view 123 --log-failed') printf '%%s' %s ;;
	*) printf 'unexpected request: %%s\n' "$*" >&2; exit 99 ;;
esac
`, quote(calls), quote("#794 Example\nbranch: "+prBranch+" at "+prHead+"\nhttps://github.com/example/repo/pull/794"), quote(prHead), quote(prBranch), quote(prJSON), quote(failed), quote(checkJSON), checkExit, quote(logs.String()))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o600))
	//nolint:gosec // This private fixture must execute as the gh command on PATH.
	require.NoError(t, os.Chmod(filepath.Join(bin, "gh"), 0o700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FIX_PR_LOG_LINES", "3")
	nested := filepath.Join(root, "nested")
	require.NoError(t, os.Mkdir(nested, 0o700))
	t.Chdir(nested)
	before := git("status", "--porcelain") + git("show-ref") + git("ls-files", "--stage")
	var oldOut, oldErr bytes.Buffer
	shell := exec.CommandContext(t.Context(), "bash", legacy, "794")
	shell.Stdout = &oldOut
	shell.Stderr = &oldErr
	shell.Env = os.Environ()
	oldCode := diagnosisProcessStatus(t, shell.Run())
	var out, stderr bytes.Buffer
	d := diagnosis{run: diagnosisRunner(t.Context()), out: &out, stderr: &stderr}
	code := d.diagnose([]string{"794"}, "3")
	assertDiagnosisParity(t, scenario, oldCode, oldOut.String(), oldErr.String(), code, out.String(), stderr.String())
	require.Equal(t, before, git("status", "--porcelain")+git("show-ref")+git("ls-files", "--stage"))
	if scenario == "failures" {
		data, err := os.ReadFile(calls)
		require.NoError(t, err)
		require.Equal(t, 2, strings.Count(string(data), "run view 123 --log-failed"))
	}
	for _, key := range []string{"LC_ALL", "GH_PAGER", "GH_PROMPT_DISABLED", "PYTHONUTF8", "CDPATH", "GH_REPO", "GH_HOST", "GREP_OPTIONS"} {
		t.Setenv(key, "hostile")
	}
	out.Reset()
	stderr.Reset()
	if hardened := (diagnosis{run: diagnosisRunner(t.Context()), out: &out, stderr: &stderr}).diagnose(
		[]string{"794"},
		"3",
	); hardened != code {
		t.Fatalf("hostile environment changed status: %d != %d", hardened, code)
	}
}

func assertDiagnosisParity(
	t *testing.T,
	scenario string,
	oldCode int,
	oldOut, oldErr string,
	code int,
	out, stderr string,
) {
	t.Helper()
	if scenario == "empty" || scenario == "unreadable" {
		require.Zero(t, oldCode)
		require.Contains(t, oldOut, "Nothing is failing")
		require.Equal(t, 2, code)
		require.NotContains(t, out, "Nothing is failing")
		return
	}
	require.Equal(t, oldCode, code)
	require.Equal(t, oldOut, out)
	require.Equal(t, oldErr, stderr)
}

func diagnosisProcessStatus(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	exit, ok := errors.AsType[*exec.ExitError](err)
	require.True(t, ok, "%v", err)
	return exit.ExitCode()
}
