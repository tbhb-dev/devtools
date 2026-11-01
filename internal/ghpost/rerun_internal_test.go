// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"bytes"
	"encoding/json"
	"flag"
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

// Each CLI case runs in a subprocess so its environment fixture can't affect
// other cases or tests running in parallel in the parent process.
func rerunCases() map[string]func(*testing.T) {
	return map[string]func(*testing.T){
		"ExactForm":                   testRerunFailedExactForm,
		"EligibleAndAudited":          testRerunFailedEligibleAndAudited,
		"StateRefusals":               testRerunFailedStateRefusals,
		"ExactRunAndIdentity":         testRerunFailedExactRunAndIdentity,
		"UncertainAndDuplicate":       testRerunFailedUncertainAndDuplicate,
		"NewAttempt":                  testRerunFailedNewAttempt,
		"ReservationAndAuditFailures": testRerunFailedReservationAndAuditFailures,
		"NoStdinAndHelp":              testRerunFailedNoStdinAndHelp,
		"PRGuard":                     testRerunFailedPRGuard,
		"Durability":                  testRerunFailedDurability,
		"SyncFailures":                testRerunFailedSyncFailures,
	}
}

func TestRerunFailed(t *testing.T) {
	t.Parallel()
	for name := range rerunCases() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			binary, err := os.Executable()
			require.NoError(t, err)
			cmd := exec.CommandContext(t.Context(), binary, "-test.run=^TestRerunFailedProcess$")
			// Merge subprocess counters into the parent test's coverage data.
			if coverageDir := flag.Lookup("test.gocoverdir").Value.String(); coverageDir != "" {
				cmd.Args = append(cmd.Args, "-test.gocoverdir="+coverageDir)
			}
			cmd.Env = append(os.Environ(), "GHPOST_RERUN_CASE="+name)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
		})
	}
}

// Only the subprocess runs environment-dependent cases. Clear the selector
// before executing the case so its child commands can't inherit it.
func TestRerunFailedProcess(t *testing.T) {
	name := os.Getenv("GHPOST_RERUN_CASE")
	if name == "" {
		return
	}
	t.Setenv("GHPOST_RERUN_CASE", "")
	test, ok := rerunCases()[name]
	require.True(t, ok, "unknown rerun case")
	test(t)
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

func testRerunFailedExactForm(t *testing.T) {
	t.Helper()
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

func testRerunFailedEligibleAndAudited(t *testing.T) {
	t.Helper()
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

func testRerunFailedStateRefusals(t *testing.T) {
	t.Helper()
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

func testRerunFailedExactRunAndIdentity(t *testing.T) {
	t.Helper()
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

func testRerunFailedUncertainAndDuplicate(t *testing.T) {
	t.Helper()
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

func testRerunFailedNewAttempt(t *testing.T) {
	t.Helper()
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

func testRerunFailedReservationAndAuditFailures(t *testing.T) {
	t.Helper()
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
				code = r.rerunFailed(t.Context(), log, &bytes.Buffer{}, &diagnostic, (*os.File).Sync)
			} else {
				code, _, _ = run(t, "", rerunArguments()...)
			}
			require.Equal(t, 2, code, diagnostic.String())
			require.NotContains(t, fileText(t, filepath.Join(dir, "calls")), "POST")
		})
	}
}

func TestRerunReservationConcurrent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := parse(rerunArguments())
	require.NoError(t, err)
	results := make(chan error, 8)
	for range 8 {
		go func() { results <- r.reserveRerun(dir, 1, (*os.File).Sync) }()
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
	require.ErrorIs(t, r.reserveRerun(dir, 1, (*os.File).Sync), errRerunReserved)
}

func testRerunFailedNoStdinAndHelp(t *testing.T) {
	t.Helper()
	setupRerun(t)
	var out, diagnostic bytes.Buffer
	code := Run(t.Context(), rerunArguments(), brokenIO{}, &out, &diagnostic)
	require.Zero(t, code, diagnostic.String())
	require.Contains(t, usage, "run rerun-failed <run-id> --repo OWNER/REPO")
}

func testRerunFailedPRGuard(t *testing.T) {
	t.Helper()
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
		output, err := cmd.CombinedOutput()
		if strings.HasPrefix(command, "gh-post ") {
			require.NoError(t, err, "%s", output)
		} else {
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit)
			require.Equal(t, 2, exit.ExitCode())
		}
	}
}

func testRerunFailedDurability(t *testing.T) {
	t.Helper()
	dir := setupRerun(t)
	stateDir := filepath.Join(dir, "state", "devtools")
	log, err := openAudit()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	r, err := parse(rerunArguments())
	require.NoError(t, err)
	var events []string
	syncFile := func(file *os.File) error {
		info, statErr := file.Stat()
		require.NoError(t, statErr)
		name := file.Name()
		if !info.IsDir() {
			// The real O_EXCL create and write have already happened.
			require.Equal(t, "reserved\n", fileText(t, file.Name()))
			events = append(events, "create")
			name = "file"
		}
		events = append(events, "sync "+name)
		// No write may precede any of the required syncs.
		require.NotContains(t, fileText(t, filepath.Join(dir, "calls")), "POST")
		return file.Sync()
	}
	var out, diagnostic bytes.Buffer
	code := r.rerunFailed(t.Context(), log, &out, &diagnostic, syncFile)
	require.Zero(t, code, diagnostic.String())
	require.Equal(t, 1, strings.Count(fileText(t, filepath.Join(dir, "calls")), "POST"))
	events = append(events, "POST")
	expected := []string{"create", "sync file"}
	if dirSyncSupported {
		for path := filepath.Join(stateDir, "gh-post-reruns", "owner", "repo", "123"); ; path = filepath.Dir(path) {
			expected = append(expected, "sync "+path)
			if filepath.Dir(path) == path {
				break
			}
		}
	}
	require.Equal(t, append(expected, "POST"), events)
}

func testRerunFailedSyncFailures(t *testing.T) {
	t.Helper()
	failures := []string{"1"}
	if dirSyncSupported {
		failures = append(failures, "123", "repo", "owner", "gh-post-reruns", "devtools", "state")
	}
	for _, failure := range failures {
		t.Run("sync-"+failure, func(t *testing.T) {
			dir := setupRerun(t)
			stateDir := filepath.Join(dir, "state", "devtools")
			log, err := openAudit()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, log.Close()) })
			r, err := parse(rerunArguments())
			require.NoError(t, err)
			syncFile := func(file *os.File) error {
				if filepath.Base(file.Name()) == failure {
					return os.ErrPermission
				}
				return file.Sync()
			}
			var out, diagnostic bytes.Buffer
			code := r.rerunFailed(t.Context(), log, &out, &diagnostic, syncFile)
			require.Equal(t, 2, code, diagnostic.String())
			require.NotContains(t, fileText(t, filepath.Join(dir, "calls")), "POST")
			require.ErrorIs(t, r.reserveRerun(stateDir, 1, (*os.File).Sync), errRerunReserved)
		})
	}
}
