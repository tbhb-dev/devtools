// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

// cspell:words CDPATH

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
)

func TestDiagnoseRefusesOutsideRepository(t *testing.T) {
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Chdir(t.TempDir())
	var out, stderr bytes.Buffer
	if code := Run([]string{"checks", "diagnose", "794"}, nil, &out, &stderr); code != 2 {
		t.Fatalf("exit = %d, want diagnosis refusal 2; stderr: %s", code, &stderr)
	}
}

type diagnosisExitError int

func (e diagnosisExitError) Error() string { return "request failed" }
func (e diagnosisExitError) ExitCode() int { return int(e) }

type diagnosisReply struct {
	data string
	err  error
}
type diagnosisFixture struct {
	replies     map[string]diagnosisReply
	calls       []string
	out, stderr bytes.Buffer
}

func newDiagnosisFixture() *diagnosisFixture {
	return &diagnosisFixture{replies: map[string]diagnosisReply{
		"git rev-parse --show-toplevel":                      {data: "/fixture\n"},
		"gh auth status":                                     {},
		"git rev-parse --abbrev-ref HEAD":                    {data: "topic\n"},
		"gh pr list --head topic --state open --json number": {data: `[{"number":794}]`},
		"gh pr view 794 --json number,title,headRefName,headRefOid,url": {
			data: `{"number":794,"title":"Example","headRefName":"topic","headRefOid":"abcdef","url":"https://github.com/example/repo/pull/794"}`,
		},
		"git rev-parse HEAD":     {data: "abcdef\n"},
		"git status --porcelain": {},
		"gh pr checks 794 --json name,state,link": {
			data: `[{"name":"test","state":"SUCCESS","link":"https://example.com/check"}]`,
		},
		"gh pr checks 794": {
			data: "test\tpass\thttps://example.com/check\n",
		},
	}}
}

func (f *diagnosisFixture) run(t *testing.T, args []string, limit string) int {
	t.Helper()
	d := diagnosis{out: &f.out, stderr: &f.stderr, run: func(dir, tool string, args ...string) ([]byte, error) {
		key := tool + " " + strings.Join(args, " ")
		f.calls = append(f.calls, key)
		if key != "git rev-parse --show-toplevel" && dir != "/fixture" {
			t.Fatalf("not root-relative: %q %s", dir, key)
		}
		reply, ok := f.replies[key]
		if !ok {
			t.Fatalf("unexpected command (writes/reproducers forbidden): %s", key)
		}
		return []byte(reply.data), reply.err
	}}
	return d.diagnose(args, limit)
}

func TestDiagnosisTargetsAndRefusals(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"794"}, {""}} {
		f := newDiagnosisFixture()
		if code := f.run(t, args, ""); code != 0 {
			t.Fatalf("%v: %d %s", args, code, &f.stderr)
		}
		if !strings.Contains(
			f.out.String(),
			"#794 Example\nbranch: topic at abcdef\nhttps://github.com/example/repo/pull/794\n",
		) {
			t.Fatal(&f.out)
		}
		discovery := slices.Contains(f.calls, "gh pr list --head topic --state open --json number")
		require.Equal(t, len(args) == 0 || args[0] == "", discovery)
	}
	for _, key := range []string{
		"git rev-parse --show-toplevel", "gh auth status", "git rev-parse --abbrev-ref HEAD",
		"gh pr list --head topic --state open --json number",
		"gh pr view 794 --json number,title,headRefName,headRefOid,url", "git rev-parse HEAD", "git status --porcelain",
	} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			f := newDiagnosisFixture()
			f.replies[key] = diagnosisReply{err: diagnosisExitError(2)}
			if code := f.run(t, nil, ""); code != 2 || !strings.HasPrefix(f.stderr.String(), "diagnose: ") {
				t.Fatalf("%d %s", code, &f.stderr)
			}
			require.Equal(t, 1, strings.Count(strings.Join(f.calls, "\n"), key))
		})
	}
	missing := newDiagnosisFixture()
	missing.replies["gh auth status"] = diagnosisReply{err: &exec.Error{Name: "gh", Err: exec.ErrNotFound}}
	require.Equal(t, 2, missing.run(t, nil, ""))
	require.Equal(t, "diagnose: gh is not installed\n", missing.stderr.String())
	for _, args := range [][]string{{"a"}, {"0"}, {"-1"}, {"1", "2"}} {
		f := newDiagnosisFixture()
		require.Equal(t, 2, f.run(t, args, ""), "%v", args)
	}
}

func TestDiagnosisMalformedData(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"gh pr list --head topic --state open --json number", "gh pr view 794 --json number,title,headRefName,headRefOid,url", "gh pr checks 794 --json name,state,link"} {
		for _, data := range []string{"", "null", "[]", "{}", "{", `[{"number":0}]`, `[{"name":"test"}]`} {
			t.Run(key+data, func(t *testing.T) {
				t.Parallel()
				f := newDiagnosisFixture()
				f.replies[key] = diagnosisReply{data: data}
				if code := f.run(t, nil, ""); code != 2 || strings.Contains(f.out.String(), "Nothing is failing") {
					t.Fatalf("%d %s %s", code, &f.out, &f.stderr)
				}
			})
		}
	}
	for _, data := range []string{`{"number":794,"title":"Example","headRefName":"topic","headRefOid":"abcdef"}`, `{"number":794,"title":"","headRefName":"topic","headRefOid":"abcdef","url":"x"}`} {
		f := newDiagnosisFixture()
		f.replies["gh pr view 794 --json number,title,headRefName,headRefOid,url"] = diagnosisReply{data: data}
		if f.run(t, nil, "") != 2 {
			t.Fatal(data)
		}
	}
	for _, status := range []diagnosisExitError{1, 2, 8} {
		f := newDiagnosisFixture()
		key := "gh pr checks 794 --json name,state,link"
		reply := f.replies[key]
		reply.err = status
		f.replies[key] = reply
		if code := f.run(t, nil, ""); code != 2 || strings.Contains(f.out.String(), "Nothing is failing") {
			t.Fatalf("%d %s", code, &f.out)
		}
	}
}

func TestDiagnosisCheckoutContext(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ branch, head, want, absent string }{
		{"topic", "abcdef", "in sync with the commit CI tested", "MISMATCH"},
		{"other", "abcdef", "the pull request is on topic, this worktree is on other.", "local HEAD differs"},
		{"topic", "123456", "local HEAD differs from the commit CI tested.", "Check out that branch"},
		{"other", "123456", "the pull request is on topic, this worktree is on other.", "local HEAD differs"},
		{"HEAD", "123456", "the pull request is on topic, this worktree is on HEAD.", "local HEAD differs"},
	} {
		f := newDiagnosisFixture()
		f.replies["git rev-parse --abbrev-ref HEAD"] = diagnosisReply{data: tc.branch}
		f.replies["git rev-parse HEAD"] = diagnosisReply{data: tc.head}
		f.replies["git status --porcelain"] = diagnosisReply{data: " M tracked\n?? untracked\n"}
		if f.run(t, []string{"794"}, "") != 0 || !strings.Contains(f.out.String(), tc.want) ||
			strings.Contains(f.out.String(), tc.absent) ||
			!strings.Contains(f.out.String(), "uncommitted changes present:\n M tracked\n?? untracked\n") {
			t.Fatal(&f.out)
		}
	}
}

func TestDiagnosisCheckSelection(t *testing.T) {
	t.Parallel()
	states := []string{
		"FAILURE",
		"TIMED_OUT",
		"CANCELLED",
		"ACTION_REQUIRED",
		"SUCCESS",
		"SKIPPED",
		"NEUTRAL",
		"PENDING",
		"IN_PROGRESS",
		"QUEUED",
		"WAITING",
		"REQUESTED",
		"STALE",
		"STARTUP_FAILURE",
	}
	checks := make([]diagnosisCheck, 0, len(states))
	for _, state := range states {
		checks = append(checks, diagnosisCheck{Name: state, State: state, Link: "https://example.com/" + state})
	}
	data, err := json.Marshal(checks)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []error{nil, diagnosisExitError(1), diagnosisExitError(8)} {
		f := newDiagnosisFixture()
		f.replies["gh pr checks 794 --json name,state,link"] = diagnosisReply{data: string(data), err: status}
		if f.run(t, []string{"794"}, "") != 1 {
			t.Fatal(&f.stderr)
		}
		for i, state := range states {
			require.Equal(t, i < 4, strings.Contains(f.out.String(), "job:        "+state+"\n"))
		}
		if !strings.HasSuffix(
			f.out.String(),
			"does not reproduce locally is a different problem from one that does.\n",
		) {
			t.Fatal(&f.out)
		}
	}
	for _, state := range states[4:] {
		f := newDiagnosisFixture()
		f.replies["gh pr checks 794 --json name,state,link"] = diagnosisReply{
			data: fmt.Sprintf(`[{"name":"check","state":%q,"link":"https://example.com/check"}]`, state),
		}
		if f.run(t, nil, "") != 0 || !strings.Contains(f.out.String(), "Nothing is failing on #794.\n") ||
			strings.Contains(f.out.String(), "all checks passed") ||
			!strings.HasSuffix(f.out.String(), f.replies["gh pr checks 794"].data) {
			t.Fatal(&f.out)
		}
	}
}

func TestDiagnosisChecksWithoutLinks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, data string
		code       int
	}{
		{"passing", `[{"name":"status","state":"SUCCESS","link":""}]`, 0},
		{"mixed failure", `[{"name":"status","state":"SUCCESS"},{"name":"lint","state":"FAILURE","link":"https://example.com/check"}]`, 1},
		{"failure without link", `[{"name":"status","state":"FAILURE","link":""}]`, 1},
		{"missing name", `[{"state":"SUCCESS","link":""}]`, 2},
		{"missing state", `[{"name":"status","link":""}]`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newDiagnosisFixture()
			f.replies["gh pr checks 794 --json name,state,link"] = diagnosisReply{data: tc.data}
			require.Equal(t, tc.code, f.run(t, nil, ""), f.stderr.String())
			switch tc.code {
			case 0:
				require.Contains(t, f.out.String(), "Nothing is failing on #794.")
			case 1:
				require.Contains(t, f.out.String(), "job:        ")
				require.NotContains(t, f.out.String(), "Nothing is failing")
			}
		})
	}
}

func TestDiagnosisLogBoundsAndDeduplication(t *testing.T) {
	t.Parallel()
	for _, limit := range []string{"", "3", "0", "invalid", "-1"} {
		f := newDiagnosisFixture()
		f.replies["gh pr checks 794 --json name,state,link"] = diagnosisReply{data: `[
	{"name":"test","state":"FAILURE","link":"https://github.com/example/repo/actions/runs/123/job/1"},
	{"name":"shell","state":"CANCELLED","link":"https://github.com/example/repo/actions/runs/123/job/2"},
	{"name":"lint","state":"TIMED_OUT","link":"https://github.com/example/repo/actions/runs/456"},
	{"name":"unknown","state":"ACTION_REQUIRED","link":"https://example.com/external"},
	{"name":"race","state":"FAILURE","link":"https://github.com/example/repo/actions/runs/789"}]`}
		var log strings.Builder
		for i := 1; i <= 100; i++ {
			fmt.Fprintf(&log, "line %d\n", i)
		}
		f.replies["gh run view 123 --log-failed"] = diagnosisReply{data: log.String()}
		f.replies["gh run view 456 --log-failed"] = diagnosisReply{data: "partial", err: diagnosisExitError(1)}
		f.replies["gh run view 789 --log-failed"] = diagnosisReply{}
		require.Equal(t, 1, f.run(t, nil, limit), "%s", &f.stderr)
		output := f.out.String()
		for _, want := range []string{"already printed above for run 123", "(not a workflow run; open the link)", "log:        empty.", "reproduce:  mise run test", "(no local task maps to this job; read the log)"} {
			require.Contains(t, output, want)
		}
		bound := 80
		if limit == "3" {
			bound = 3
		}
		if limit == "0" {
			bound = 0
		}
		if !strings.Contains(output, fmt.Sprintf("last %d lines of the failing steps:\n", bound)) ||
			strings.Count(output, "\nline ") != bound ||
			strings.Contains(output, "partial") {
			t.Fatal(output)
		}
		for _, id := range []string{"123", "456", "789"} {
			require.Equal(t, 1, strings.Count(strings.Join(f.calls, "\n"), "gh run view "+id+" --log-failed"))
		}
	}
}

func TestDiagnosisReproducers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ patterns, task string }{
		{"prose,vale", "mise run lint-prose"},
		{"spell", "mise run devtools:lint-spelling"},
		{"markdown,rumdl", "mise run devtools:lint-markdown"},
		{"shell", "mise run devtools:lint-shell && mise run devtools:lint-shell-fmt"},
		{"yaml", "mise run devtools:lint-yaml"},
		{"toml", "mise run devtools:lint-toml"},
		{"workflow,actionlint", "mise run devtools:lint-workflows"},
		{"editorconfig", "mise run lint-editorconfig"},
		{"arch", "mise run devtools:lint-go-arch"},
		{"deadcode", "mise run devtools:lint-go-deadcode"},
		{"lint", "mise run lint"},
		{"race", "mise run devtools:test-go-race"},
		{"cover", "mise run devtools:cover-go-check"},
		{"test,test (linux),test (macos),test (windows)", "mise run test"},
		{"vendor", "mise run devtools:vendor-check"},
		{"vuln", "mise run devtools:vuln"},
		{"gitleaks,secret", "mise run devtools:gitleaks"},
		{"apm,validate", "apm install --frozen && apm audit --ci"},
		{"fuzz", "mise run fuzz"},
		{"unknown job,L\u0130NT", "(no local task maps to this job; read the log)"},
		{"vale shell test", "mise run lint-prose"},
		{"shell lint", "mise run devtools:lint-shell && mise run devtools:lint-shell-fmt"},
		{"lint race", "mise run lint"},
		{"test vendor", "mise run test"},
	} {
		for pattern := range strings.SplitSeq(tc.patterns, ",") {
			for _, job := range []string{pattern, strings.ToUpper(pattern)} {
				if got := diagnosisReproducer(job); got != tc.task {
					t.Fatalf("%s: %s != %s", job, got, tc.task)
				}
			}
		}
		assertDiagnosisTasks(t, tc.task)
	}
}

func assertDiagnosisTasks(t *testing.T, suggestion string) {
	t.Helper()
	files, err := filepath.Glob("../../.devtools/tasks/*.toml")
	if err != nil {
		t.Fatal(err)
	}
	shared := map[string]any{}
	for _, path := range files {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		tasks := map[string]any{}
		require.NoError(t, toml.Unmarshal(data, &tasks))
		for task := range tasks {
			shared[task] = true
		}
	}
	data, err := os.ReadFile("../../mise.toml")
	if err != nil {
		t.Fatal(err)
	}
	var repo struct {
		Tasks map[string]any `toml:"tasks"`
	}
	require.NoError(t, toml.Unmarshal(data, &repo))
	words := strings.Fields(suggestion)
	for i, word := range words {
		if word == "run" && i > 0 && words[i-1] == "mise" {
			task := words[i+1]
			require.True(t, shared[task] != nil || repo.Tasks[task] != nil, "undefined task %s", task)
			require.Nil(t, shared["devtools:"+task], "prefer shared task %s", task)
		}
	}
}

func TestDiagnosisEnvironmentAndProbe(t *testing.T) {
	t.Setenv("GH_REPO", "hostile")
	for _, key := range []string{"LC_ALL", "GH_PAGER", "GH_PROMPT_DISABLED", "PYTHONUTF8", "CDPATH", "GH_REPO", "GH_HOST", "GREP_OPTIONS"} {
		t.Setenv(key, "hostile")
	}
	env := strings.Join(diagnosisEnv(), "\n")
	if strings.Contains(env, "hostile") {
		t.Fatal(env)
	}
	for _, entry := range []string{"LC_ALL=C", "GH_PAGER=cat", "GH_PROMPT_DISABLED=1", "PYTHONUTF8=1"} {
		if !strings.Contains(env, entry) {
			t.Fatal(env)
		}
	}
	t.Chdir(t.TempDir())
	var out, stderr bytes.Buffer
	if code := Run(
		[]string{"checks", "diagnose", "--help"},
		nil,
		&out,
		&stderr,
	); code != 0 || !strings.Contains(out.String(), "Exit codes:") ||
		stderr.Len() != 0 {
		t.Fatalf("%d %s %s", code, &out, &stderr)
	}
}

func TestDiagnosisDisplayRefusal(t *testing.T) {
	t.Parallel()
	for _, reply := range []diagnosisReply{{}, {data: "partial", err: diagnosisExitError(2)}} {
		f := newDiagnosisFixture()
		f.replies["gh pr checks 794"] = reply
		require.Equal(t, 2, f.run(t, nil, ""))
		require.Contains(t, f.stderr.String(), "cannot display checks")
		require.NotContains(t, f.out.String(), "Nothing is failing")
	}
}
