// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//nolint:paralleltest // The recording gh fixture changes the process environment.
package ghpost

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const rerunURL = "https://github.com/owner/repo/actions/runs/123"

func rerunArguments() []string {
	return []string{"run", "rerun-failed", "123", "--repo", "owner/repo"}
}

func setupRerun(t *testing.T) string {
	t.Helper()
	dir := setup(t)
	t.Setenv("RERUN_DIR", dir)
	t.Setenv("RERUN_ERROR", "0")
	writeRun(t, dir, "before", "completed", "failure", 1)
	writeRun(t, dir, "after", "queued", "", 2)
	write(t, filepath.Join(dir, "gh"), `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
case "$*" in
	'api user --jq .login') echo "${LOGIN:-agent}"; exit "${IDENTITY_STATUS:-0}" ;;
	'api graphql '*) echo 'app[bot]'; exit "${GRAPHQL_STATUS:-1}" ;;
	'api --method GET repos/owner/repo/actions/runs/123')
		if [ -f "$RERUN_DIR/posted" ]; then cat "$RERUN_DIR/after"; else cat "$RERUN_DIR/before"; fi ;;
	'api --method POST repos/owner/repo/actions/runs/123/rerun-failed-jobs')
		touch "$RERUN_DIR/posted"
		echo 'ghp_abcdefghijklmnopqrstuvwxyz1234567890' >&2
		exit "$RERUN_ERROR" ;;
	*) exit 99 ;;
esac
`)
	return dir
}

func writeRun(t *testing.T, dir, file, status, conclusion string, attempt uint64) {
	t.Helper()
	data, err := json.Marshal(
		workflowRun{ID: 123, Attempt: attempt, Status: status, Conclusion: conclusion, URL: rerunURL},
	)
	require.NoError(t, err)
	write(t, filepath.Join(dir, file), string(data))
}

func TestRerunFailedExactForm(t *testing.T) {
	for _, args := range [][]string{
		{"run"},
		{"run", "rerun", "123", "--repo", "owner/repo"},
		{"run", "cancel", "123", "--repo", "owner/repo"},
		{"run", "rerun-failed", "123"},
		{"run", "rerun-failed", "123", "-R", "owner/repo"},
		{"run", "rerun-failed", "123", "--repo=owner/repo"},
		{"run", "rerun-failed", "123", "--repo", "https://github.com/owner/repo"},
		{"run", "rerun-failed", "123", "--repo", "owner/repo", "--repo", "other/repo"},
		{"run", "rerun-failed", "123", "--repo", "owner/repo", "--body-file", "-"},
		{"run", "rerun-failed", "123", "--repo", "owner/repo", "--failed"},
		{"run", "rerun-failed", "123", "--repo", "owner/repo", "--job", "456"},
		{"run", "rerun-failed", "123", "--repo", "owner/repo", "--dry-run"},
		{"run", "rerun-failed", "0", "--repo", "owner/repo"},
		{"run", "rerun-failed", "0123", "--repo", "owner/repo"},
		{"run", "rerun-failed", "-123", "--repo", "owner/repo"},
		{"run", "rerun-failed", "123/attempts/1", "--repo", "owner/repo"},
		{"run", "rerun-failed", "18446744073709551616", "--repo", "owner/repo"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := setupRerun(t)
			code, _, diagnostic := run(t, "", args...)
			require.Equal(t, 2, code, diagnostic)
			require.NoFileExists(t, filepath.Join(dir, "calls"))
		})
	}
}

func TestRerunFailedEligibleAndAudited(t *testing.T) {
	for _, conclusion := range []string{"failure", "cancelled", "timed_out"} {
		t.Run(conclusion, func(t *testing.T) {
			dir := setupRerun(t)
			writeRun(t, dir, "before", "completed", conclusion, 7)
			code, out, diagnostic := run(t, "ignored", rerunArguments()...)
			require.Equal(t, 0, code, diagnostic)
			require.Equal(t, rerunURL+"\n", out)
			require.Empty(t, diagnostic)
			require.Equal(t, "api user --jq .login\napi --method GET repos/owner/repo/actions/runs/123\n"+
				"api --method POST repos/owner/repo/actions/runs/123/rerun-failed-jobs\n", fileText(t, filepath.Join(dir, "calls")))
			var record struct {
				auditRecord

				Attempt uint64 `json:"run_attempt"`
				Outcome string `json:"outcome"`
			}
			audit := fileText(t, filepath.Join(dir, "state", "devtools", "gh-post.log"))
			require.NoError(t, json.Unmarshal([]byte(audit), &record))
			require.Equal(t, runRerunFailed, record.Operation)
			require.Equal(t, rerunURL, record.URL)
			require.Equal(t, uint64(7), record.Attempt)
			require.Equal(t, "reserved", record.Outcome)
			_, err := time.Parse(time.RFC3339Nano, record.Time)
			require.NoError(t, err)
			require.NotContains(t, audit, "ghp_")
		})
	}
}

func TestRerunFailedStateRefusals(t *testing.T) {
	for _, pair := range [][2]string{
		{"completed", "success"},
		{"completed", "skipped"},
		{"completed", "neutral"},
		{"completed", "action_required"},
		{"completed", "stale"},
		{"completed", ""},
		{"queued", "failure"},
		{"in_progress", "failure"},
		{"waiting", ""},
	} {
		t.Run(pair[0]+"-"+pair[1], func(t *testing.T) {
			dir := setupRerun(t)
			writeRun(t, dir, "before", pair[0], pair[1], 1)
			code, _, diagnostic := run(t, "", rerunArguments()...)
			require.Equal(t, 1, code, diagnostic)
			require.Contains(t, diagnostic, `status="`+pair[0]+`" conclusion="`+pair[1]+`"`)
			require.NotContains(t, fileText(t, filepath.Join(dir, "calls")), "POST")
		})
	}
}

func TestRerunFailedExactRunAndIdentity(t *testing.T) {
	for _, body := range []string{
		`{}`, `invalid`,
		`{"id":456,"run_attempt":1,"status":"completed","conclusion":"failure","html_url":"` + rerunURL + `"}`,
		`{"id":123,"run_attempt":1,"status":"completed","conclusion":"failure","html_url":"https://github.com/other/repo/actions/runs/123"}`,
		`{"id":123,"run_attempt":0,"status":"completed","conclusion":"failure","html_url":"` + rerunURL + `"}`,
	} {
		t.Run(body, func(t *testing.T) {
			dir := setupRerun(t)
			write(t, filepath.Join(dir, "before"), body)
			code, _, diagnostic := run(t, "", rerunArguments()...)
			require.Equal(t, 2, code, diagnostic)
			require.NotContains(t, fileText(t, filepath.Join(dir, "calls")), "POST")
		})
	}
	for _, login := range []string{"outsider", "unreadable"} {
		t.Run(login, func(t *testing.T) {
			dir := setupRerun(t)
			t.Setenv("LOGIN", login)
			if login == "unreadable" {
				t.Setenv("IDENTITY_STATUS", "1")
			}
			code, _, diagnostic := run(t, "", rerunArguments()...)
			require.NotZero(t, code, diagnostic)
			require.NotContains(t, fileText(t, filepath.Join(dir, "calls")), "actions/runs")
		})
	}
}

func TestRerunFailedUncertainAndDuplicate(t *testing.T) {
	for _, after := range []string{"accepted", "unchanged", "unreadable"} {
		t.Run(after, func(t *testing.T) {
			dir := setupRerun(t)
			t.Setenv("RERUN_ERROR", "1")
			switch after {
			case "unchanged":
				writeRun(t, dir, "after", "completed", "failure", 1)
			case "unreadable":
				write(t, filepath.Join(dir, "after"), "invalid")
			}
			code, _, diagnostic := run(t, "", rerunArguments()...)
			require.Equal(t, 2, code, diagnostic)
			require.Contains(t, diagnostic, "outcome uncertain; do not retry")
			require.NotContains(t, diagnostic, "ghp_")
			switch after {
			case "accepted":
				require.Contains(t, diagnostic, `status="queued" conclusion="" attempt=2`)
			case "unchanged":
				require.Contains(t, diagnostic, `status="completed" conclusion="failure" attempt=1`)
			default:
				require.Contains(t, diagnostic, "cannot read run back")
			}
			calls := fileText(t, filepath.Join(dir, "calls"))
			require.Equal(t, 2, strings.Count(calls, "GET repos/"))
			require.Equal(t, 1, strings.Count(calls, "POST repos/"))
			// A fresh invocation sees the old attempt again, even if the
			// read-back was delayed or unavailable after the first POST.
			require.NoError(t, os.Remove(filepath.Join(dir, "posted")))
			code, _, diagnostic = run(t, "", rerunArguments()...)
			require.Equal(t, 1, code, diagnostic)
			require.Contains(t, diagnostic, "already reserved")
			require.Equal(t, 1, strings.Count(fileText(t, filepath.Join(dir, "calls")), "POST repos/"))
		})
	}
}

func TestRerunFailedNewAttempt(t *testing.T) {
	dir := setupRerun(t)
	code, _, diagnostic := run(t, "", rerunArguments()...)
	require.Zero(t, code, diagnostic)
	require.NoError(t, os.Remove(filepath.Join(dir, "posted")))
	code, _, diagnostic = run(t, "", rerunArguments()...)
	require.Equal(t, 1, code, diagnostic)
	writeRun(t, dir, "before", "completed", "failure", 2)
	code, _, diagnostic = run(t, "", rerunArguments()...)
	require.Zero(t, code, diagnostic)
	require.Equal(t, 2, strings.Count(fileText(t, filepath.Join(dir, "calls")), "POST repos/"))
}

func TestRerunFailedReservationAndAuditFailures(t *testing.T) {
	for _, failure := range []string{"reservation", "audit-open", "audit-append"} {
		t.Run(failure, func(t *testing.T) {
			dir := setupRerun(t)
			stateDir := filepath.Join(dir, "state", "devtools")
			var code int
			var diagnostic bytes.Buffer
			if failure == "audit-open" {
				require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "gh-post.log"), 0o700))
			}
			if failure == "reservation" {
				write(t, filepath.Join(stateDir, "gh-post-reruns"), "not a directory")
			}
			if failure == "audit-append" {
				log, err := openAudit()
				require.NoError(t, err)
				require.NoError(t, log.Close())
				r, err := parse(rerunArguments())
				require.NoError(t, err)
				code = r.rerunFailed(t.Context(), log, &bytes.Buffer{}, &diagnostic)
			} else {
				code, _, _ = run(t, "", rerunArguments()...)
			}
			require.Equal(t, 2, code, diagnostic.String())
			require.NotContains(t, fileText(t, filepath.Join(dir, "calls")), "POST")
		})
	}
}

func TestRerunReservationConcurrent(t *testing.T) {
	dir := t.TempDir()
	r, err := parse(rerunArguments())
	require.NoError(t, err)
	results := make(chan error, 8)
	for range 8 {
		go func() { results <- r.reserveRerun(dir, 1) }()
	}
	successes := 0
	for range 8 {
		err = <-results
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, errRerunReserved)
		}
	}
	require.Equal(t, 1, successes)
	r.repo = "OWNER/REPO"
	require.ErrorIs(t, r.reserveRerun(dir, 1), errRerunReserved)
}

func TestRerunFailedNoStdinAndHelp(t *testing.T) {
	setupRerun(t)
	var out, diagnostic bytes.Buffer
	code := Run(t.Context(), rerunArguments(), brokenIO{}, &out, &diagnostic)
	require.Zero(t, code, diagnostic.String())
	require.Contains(t, usage, "run rerun-failed <run-id> --repo OWNER/REPO")
}

func TestRerunFailedPRGuard(t *testing.T) {
	setup(t)
	for _, command := range []string{
		"gh-post run rerun-failed 123 --repo owner/repo",
		"gh pr edit 12 --title example",
	} {
		payload, err := json.Marshal(map[string]any{"tool_input": map[string]string{"command": command}})
		require.NoError(t, err)
		cmd := exec.CommandContext(
			t.Context(),
			"bash",
			"../../packages/agents/source/skills/pr/claude/scripts/guard-gh.sh",
		)
		cmd.Stdin = bytes.NewReader(payload)
		err = cmd.Run()
		if strings.HasPrefix(command, "gh-post ") {
			require.NoError(t, err)
		} else {
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit)
			require.Equal(t, 2, exit.ExitCode())
		}
	}
}
