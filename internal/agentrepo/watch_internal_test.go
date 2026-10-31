// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestWatcherSnapshots(t *testing.T) {
	t.Parallel()
	check := func(name, bucket, workflow, id string) string {
		return fmt.Sprintf(
			`{"name":%q,"bucket":%q,"workflow":%q,"link":"https://example.com/job/%s"}`,
			name,
			bucket,
			workflow,
			id,
		)
	}
	pass := check("build", "pass", "ci", "9")
	pending := check("build", "pending", "ci", "5")
	fail := check("build", "fail", "ci", "5")
	cancel := check("other", "cancel", "ci", "6")
	for _, tt := range []struct {
		name      string
		snapshots []string
		status    int
		output    string
		reads     int
		sleeps    []int
	}{
		{"single read per snapshot", []string{"[" + pending + "]", "[" + pending + "]", "[" + pending + "]", "[" + pass + "]"}, 0, "PASS  build\nALL GREEN #7\n", 4, []int{3, 3}},
		{"newest attempt", []string{"[" + pass + "," + fail + "]"}, 0, "PASS  build\nALL GREEN #7\n", 2, nil},
		{"workflows", []string{"[" + pass + "," + check("build", "fail", "other", "6") + "]"}, 1, "FAIL  build  https://example.com/job/6\nPASS  build\nFAILED #7 with 1 failing check(s). Hand it to the fix-pr skill.\n", 2, nil},
		{"recovery", []string{"[" + pending + "]", "[" + fail + "," + check("slow", "pending", "ci", "7") + "]", "[" + pass + "]"}, 0, "FAIL  build  https://example.com/job/5\nALL GREEN #7\n", 3, []int{3}},
		{"deferred cancellation", []string{"[" + pending + "]", "[" + pending + "," + cancel + "]", "[" + pass + "," + cancel + "]"}, 1, "FAIL  other  https://example.com/job/6\nPASS  build\nFAILED #7 with 1 failing check(s). Hand it to the fix-pr skill.\n", 3, []int{3}},
		{"cancel superseded", []string{"[" + pending + "]", "[" + pending + "," + cancel + "]", "[" + pass + "," + check("other", "pass", "ci", "8") + "]"}, 0, "PASS  build\nPASS  other\nALL GREEN #7\n", 3, []int{3}},
		{"literal buckets", []string{"[" + check("skip !", "skipping", "ci", "1") + "," + check("odd", "unknown", "ci", "2") + "]"}, 0, "SKIP  skip !\nunknown  odd\nALL GREEN #7\n", 2, nil},
		{"empty", []string{"[]"}, 2, "NO CHECKS #7 registered after 0s\n", 1, nil},
		{"empty while settling", []string{"[" + pending + "]", "[]", "[" + pass + "]"}, 0, "PASS  build\nALL GREEN #7\n", 3, []int{3}},
		{"empty until timeout", []string{"[" + pending + "]", "[]"}, 2, "TIMEOUT #7 after 6s with checks still running\n", 4, []int{3, 3}},
		{"link without digits", []string{`[{"name":"build","bucket":"pass","link":"https://example.com/","workflow":"ci"}]`}, 0, "PASS  build\nALL GREEN #7\n", 2, nil},
		{"late invalid", []string{`[{"name":"build","bucket":"pass","link":"/1","workflow":"ci"}]`, "{"}, 2, "ERROR gh returned an invalid checks response for #7\n", 2, nil},
		{"invalid", []string{"{"}, 2, "ERROR gh returned an invalid checks response for #7\n", 1, nil},
		{"non array", []string{"{}"}, 2, "ERROR gh returned an invalid checks response for #7\n", 1, nil},
		{"processing", []string{`[1]`}, 2, "ERROR could not normalize checks for #7\n", 1, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, stderr bytes.Buffer
			reads := 0
			var sleeps []int
			w := checkWatcher{
				out:      &out,
				stderr:   &stderr,
				interval: 3,
				timeout:  6,
				sleep:    func(n int) { sleeps = append(sleeps, n) },
				read: func() ([]byte, string, int) {
					i := reads
					reads++
					if i >= len(tt.snapshots) {
						i = len(tt.snapshots) - 1
					}
					return []byte(tt.snapshots[i]), "", 0
				},
			}
			status := w.watch("7")
			if status != tt.status || out.String() != "WATCHING #7\n"+tt.output || reads != tt.reads ||
				fmt.Sprint(sleeps) != fmt.Sprint(tt.sleeps) {
				t.Fatalf("status=%d reads=%d sleeps=%v\n%s\nstderr=%s", status, reads, sleeps, &out, &stderr)
			}
		})
	}
}

func TestWatchWaitBounds(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name                          string
		appearance, interval, timeout int
		empty                         int
		output                        string
		reads                         int
		sleeps                        []int
	}{
		{"default appearance", 120, 30, 1800, 100, "NO CHECKS #7 registered after 120s\n", 25, repeatWatchSleep(5, 24)},
		{"appearance override", 10, 30, 1800, 100, "NO CHECKS #7 registered after 10s\n", 3, []int{5, 5}},
		{"appearance independent", 10, 3, 6, 1, "TIMEOUT #7 after 6s with checks still running\n", 6, []int{5, 3, 3}},
		{"default settling", 0, 30, 1800, 0, "TIMEOUT #7 after 1800s with checks still running\n", 62, repeatWatchSleep(30, 60)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			var sleeps []int
			reads := 0
			w := checkWatcher{
				out:        &out,
				stderr:     &out,
				appearance: tt.appearance,
				interval:   tt.interval,
				timeout:    tt.timeout,
				sleep:      func(n int) { sleeps = append(sleeps, n) },
				read: func() ([]byte, string, int) {
					reads++
					if reads <= tt.empty {
						return []byte("[]"), "", 0
					}
					return []byte(`[{"name":"wait","workflow":"ci","bucket":"pending","link":"/1"}]`), "", 0
				},
			}
			status := w.watch("7")
			if status != 2 || !strings.HasSuffix(out.String(), tt.output) || reads != tt.reads ||
				fmt.Sprint(sleeps) != fmt.Sprint(tt.sleeps) {
				t.Fatalf("status=%d reads=%d sleeps=%v output=%s", status, reads, sleeps, &out)
			}
		})
	}
}

func repeatWatchSleep(value, count int) []int {
	result := make([]int, count)
	for i := range result {
		result[i] = value
	}
	return result
}

func TestWatchReadFailures(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, diagnostic string
		status, want     int
		output           string
	}{
		{"ordinary", "HTTP 502: Bad Gateway\n", 1, 2, "ERROR could not read checks for #7: HTTP 502: Bad Gateway\n"},
		{"auth", "HTTP 401: Bad credentials\n", 1, 2, "ERROR could not read checks for #7: HTTP 401: Bad credentials\n"},
		{"no checks", "no checks reported on the 'topic' branch\n", 1, 2, "NO CHECKS #7 registered after 0s\n"},
		{"quota", "guidance\noriginal error\n", 76, 76, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, stderr bytes.Buffer
			reads := 0
			w := checkWatcher{out: &out, stderr: &stderr, read: func() ([]byte, string, int) {
				reads++
				return []byte(`[{"name":"pass","bucket":"pass","link":"/1"}]`), tt.diagnostic, tt.status
			}}
			status := w.watch("7")
			if status != tt.want || reads != 1 || out.String() != "WATCHING #7\n"+tt.output {
				t.Fatalf("status=%d reads=%d output=%s", status, reads, &out)
			}
			if tt.status == 76 && stderr.String() != tt.diagnostic {
				t.Fatalf("lost error: %s", &stderr)
			}
		})
	}
}

func TestWatchTarget(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name                      string
		args                      []string
		login, pr                 string
		failureCall, status, want int
		diagnostic, output        string
		calls                     int
	}{
		{name: "explicit", args: []string{"7"}, login: "agent\n", pr: "7\n", calls: 1},
		{name: "inferred", login: "agent\n", pr: "7\n", calls: 2},
		{name: "absent", login: "agent\n", want: 2, output: "ERROR no open pull request for topic\n", calls: 2},
		{name: "null viewer", login: "null\n", want: 1, diagnostic: "GitHub returned no viewer login\n", calls: 1},
		{name: "empty viewer", want: 1, diagnostic: "GitHub returned no viewer login\n", calls: 1},
		{name: "access", failureCall: 1, status: 4, want: 4, diagnostic: "auth failed\n", calls: 1},
		{name: "discovery failed", login: "agent\n", failureCall: 2, status: 1, want: 1, diagnostic: "lookup failed\n", calls: 2},
		{name: "quota", login: "agent\n", failureCall: 2, status: 1, want: 76, diagnostic: "gh: API rate limit exceeded for user ID 1. (HTTP 403)\n", calls: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := &cobra.Command{}
			var out, stderr bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&stderr)
			calls := 0
			number, _, status := selectWatchTarget(cmd, tt.args, func(args ...string) ([]byte, string, int) {
				calls++
				if calls == tt.failureCall {
					return nil, tt.diagnostic, tt.status
				}
				if calls == 1 {
					return []byte(tt.login), "", 0
				}
				if strings.Join(args, " ") != "pr list --head topic --state open --json number --jq .[0].number" {
					t.Fatalf("discovery args: %v", args)
				}
				return []byte(tt.pr), "", 0
			}, func() ([]byte, string, int) {
				require.Empty(t, tt.args, "explicit target read branch")
				return []byte("topic\n"), "", 0
			})
			require.Equal(t, tt.want, status)
			require.Equal(t, tt.calls, calls)
			require.Equal(t, tt.output, out.String())
			require.Equal(t, status == 0, number == "7")
			require.True(t, strings.HasSuffix(stderr.String(), tt.diagnostic), stderr.String())
			require.Equal(t, tt.want == 76, strings.Contains(stderr.String(), "rate limit:"))
		})
	}
}

func TestWatchChecksReadQuota(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, diagnostic, kind string }{
		{"primary", "gh: API rate limit exceeded for user ID 1. (HTTP 403)\n", "primary GraphQL rate limit"},
		{"secondary", "gh: You have exceeded a secondary rate limit. (HTTP 403)\n", "secondary rate limit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, stderr bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetErr(&stderr)
			var calls []string
			_, gh, status := selectWatchTarget(cmd, []string{"7"}, func(args ...string) ([]byte, string, int) {
				calls = append(calls, strings.Join(args, " "))
				if len(calls) == 1 {
					return []byte("agent\n"), "", 0
				}
				return nil, tt.diagnostic, 1
			}, nil)
			require.Zero(t, status)
			sleeps := 0
			w := checkWatcher{
				out: &out, stderr: &stderr, interval: 3, timeout: 6, sleep: func(int) { sleeps++ },
				read: func() ([]byte, string, int) { return gh("pr", "checks", "7") },
			}
			require.Equal(t, 76, w.watch("7"))
			require.Len(t, calls, 2, "the failed read isn't retried")
			require.Zero(t, sleeps)
			require.Equal(t, "WATCHING #7\n", out.String())
			require.Contains(t, stderr.String(), "rate limit: GitHub "+tt.kind)
			require.Contains(t, stderr.String(), "No automatic retry was made.")
			require.True(t, strings.HasSuffix(stderr.String(), tt.diagnostic), stderr.String())
		})
	}
}

func TestWatchQuota(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		line string
		kind string
	}{
		{"gh: secondary rate limit\n", "secondary rate limit"},
		{"gh: API rate limit already exceeded for someone\n", "primary GraphQL rate limit"},
		{"GraphQL: API rate limit exceeded for someone\n", "primary GraphQL rate limit"},
		{"gh: API rate limit exceeded for someone\n", "primary GraphQL rate limit"},
		{"API rate limit exceeded for someone\n", ""},
		{"gh: HTTP 403\n", ""},
	} {
		guidance := watchQuota(tt.line)
		if tt.kind == "" {
			if guidance != "" {
				t.Fatal(guidance)
			}
			continue
		}
		if !strings.Contains(guidance, tt.kind) || !strings.Contains(guidance, "No automatic retry was made.") {
			t.Fatal(guidance)
		}
		primary := strings.HasPrefix(tt.kind, "primary GraphQL")
		if primary && !strings.Contains(guidance, "rateLimit { remaining resetAt }") {
			t.Fatal(guidance)
		}
	}
}

func TestWatchSettingsAndEnvironment(t *testing.T) {
	t.Setenv("PR_CHECKS_INTERVAL", "")
	for _, tt := range []struct {
		value string
		want  int
	}{{"", 30}, {"7", 7}, {"bad", 30}, {"0", 0}} {
		t.Setenv("PR_CHECKS_INTERVAL", tt.value)
		if got := watchSetting("PR_CHECKS_INTERVAL", 30); got != tt.want {
			t.Fatalf("setting %q: %d", tt.value, got)
		}
	}
	for _, key := range []string{"LC_ALL", "GH_PAGER", "GH_PROMPT_DISABLED", "PYTHONUTF8", "CDPATH", "GH_REPO", "GH_HOST", "GREP_OPTIONS"} {
		t.Setenv(key, "retarget")
	}
	env := strings.Join(diagnosisEnv(), "\n")
	if strings.Contains(env, "retarget") {
		t.Fatal("inherited retargeting environment")
	}
	for _, entry := range []string{"LC_ALL=C", "GH_PAGER=cat", "GH_PROMPT_DISABLED=1", "PYTHONUTF8=1"} {
		if !strings.Contains(env, entry) {
			t.Fatal(entry)
		}
	}
}

func TestWatchStreamsBeforeNextRead(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	reads := 0
	w := checkWatcher{out: &out, stderr: &out, interval: 1, timeout: 1, sleep: func(int) {
		if !strings.Contains(out.String(), "FAIL  early  /1\n") {
			t.Fatal("event was buffered")
		}
	}, read: func() ([]byte, string, int) {
		reads++
		return []byte(
			`[{"name":"early","workflow":"ci","bucket":"fail","link":"/1"},{"name":"slow","workflow":"ci","bucket":"pending","link":"/2"}]`,
		), "", 0
	}}
	if status := w.watch("7"); status != 2 || strings.Count(out.String(), "FAIL  early") != 1 || reads != 3 {
		t.Fatalf("status=%d reads=%d output=%s", status, reads, &out)
	}
}
