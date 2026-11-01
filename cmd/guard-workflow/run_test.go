// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type commandCase struct {
	ac, skill, command string
	blocked            bool
	entry              string
}

func commandCases() []commandCase {
	cases := make([]commandCase, 0, 138)
	for _, skill := range []string{"pr", "merge-pr"} {
		entry := "create-pr.sh"
		if skill == "merge-pr" {
			entry = "squash-merge.sh"
		}
		for _, verb := range []string{"list", "view", "diff", "status", "checks"} {
			cases = append(cases, commandCase{"AC1", skill, "gh pr " + verb, false, ""})
		}
		for _, verb := range []string{"create", "edit", "ready", "merge", "close", "comment", "future-verb"} {
			cases = append(cases, commandCase{"AC2", skill, "gh pr " + verb + " 1", true, entry})
		}
		cases = append(cases,
			commandCase{"AC2", skill, "gh pr view 1; gh pr merge 1", false, ""},
			commandCase{"AC2", skill, "gh pr merge 1; gh pr view 1", true, entry},
			commandCase{"AC4", skill, `echo "gh pr merge 1"`, false, ""},
			commandCase{"AC4", skill, "cat <<'EOF'\ngh pr merge 1\nEOF", true, entry},
			commandCase{"AC4", skill, `echo "; gh pr merge 1"`, true, entry},
		)
	}
	cases = append(cases,
		commandCase{"AC5", "watch-pr", "gh pr checks 1", false, ""},
		commandCase{"AC5", "watch-pr", "gh pr checks 1 --watch", true, "watch-checks.sh"},
		commandCase{"AC5", "watch-pr", "gh pr checks 1 --fail-fast", true, "watch-checks.sh"},
		commandCase{"AC5", "watch-pr", "gh run watch 1", true, "watch-checks.sh"},
		commandCase{"AC5", "fix-pr", "gh pr checks 1", false, ""},
		commandCase{"AC5", "fix-pr", "gh pr checks 1 --watch", true, "watch-pr skill"},
		commandCase{"AC5", "fix-pr", "gh pr checks 1 --fail-fast", true, "watch-pr skill"},
		commandCase{"AC5", "fix-pr", "gh run watch 1", false, ""},
		commandCase{"AC6", "fix-pr", "gh run view 1 --log", true, "diagnose.sh"},
		commandCase{"AC6", "fix-pr", "gh run view 1 --log-failed", true, "diagnose.sh"},
		commandCase{"AC6", "fix-pr", "gh run view 1 --job 2 --log", false, ""},
		commandCase{"AC6", "fix-pr", "gh run view 1 --log-failed --job=2", false, ""},
		commandCase{"AC6", "fix-pr", "gh run view 1", false, ""},
		commandCase{"AC6", "fix-pr", "gh run view 1 --logging", false, ""},
		commandCase{"AC7", "fix-pr", "git push --force origin HEAD", true, "--force-with-lease"},
		commandCase{"AC7", "fix-pr", "git push -f", true, "--force-with-lease"},
		commandCase{"AC7", "fix-pr", "git push --force-with-lease", false, ""},
		commandCase{"AC7", "fix-pr", "git push --force --force-with-lease", false, ""},
		commandCase{"AC7", "fix-pr", "git push --forceful", false, ""},
		commandCase{"AC7", "fix-pr", "git push -foo", false, ""},
		commandCase{"AC4", "watch-pr", `gh pr checks 1; echo "--watch"`, true, "watch-checks.sh"},
		commandCase{"AC4", "fix-pr", "gh pr checks 1\ncat <<EOF\n--fail-fast\nEOF", true, "watch-pr skill"},
		commandCase{"AC4", "fix-pr", "gh run view 1\ncat <<EOF\n--log\nEOF", true, "diagnose.sh"},
		commandCase{"AC4", "fix-pr", `gh run view 1 --log; echo "--job"`, false, ""},
		commandCase{"AC4", "fix-pr", "git push\ncat <<EOF\n--force\nEOF", true, "--force-with-lease"},
		commandCase{"AC4", "fix-pr", `git push -f; echo "--force-with-lease"`, false, ""},
		commandCase{"AC4", "fix-pr", `gh run view 1; echo "--log"`, false, ""},
		commandCase{"AC4", "fix-pr", `gh run view 1; echo "--log-failed"`, true, "diagnose.sh"},
		commandCase{"AC4", "fix-pr", `git push; echo "--force"`, false, ""},
		commandCase{"AC4", "fix-pr", `git push; echo "--force "`, true, "--force-with-lease"},
		commandCase{"AC4", "fix-pr", "gh run view 1 --log ; git push -f ; gh pr checks --watch", true, "diagnose.sh"},
		commandCase{"AC4", "fix-pr", "git push -f ; gh pr checks --watch", true, "--force-with-lease"},
	)
	for _, call := range []struct{ skill, command string }{
		{"pr", "gh pr merge 1"},
		{"merge-pr", "gh pr merge 1"},
		{"watch-pr", "gh run watch 1"},
		{"fix-pr", "git push -f"},
		{"fix-pr", "gh run view 1 --log"},
		{"fix-pr", "gh pr checks --watch"},
	} {
		for _, prefix := range []string{"", " \t", "; ", "& ", "| ", "( ", "&& ", "|| ", "\n", "\r\v\f"} {
			cases = append(cases, commandCase{"AC4", call.skill, prefix + call.command, true, ""})
		}
		cases = append(cases,
			commandCase{"AC4", call.skill, "echo " + call.command, false, ""},
			commandCase{"AC4", call.skill, "echo \"text\n" + call.command + " \"", true, ""},
		)
	}
	return cases
}

type result struct {
	code           int
	stdout, stderr string
}

func execute(input string, args ...string) result {
	var out, errOut bytes.Buffer
	code := Run(args, streams{in: strings.NewReader(input), out: &out, err: &errOut})
	return result{code, out.String(), errOut.String()}
}

func payload(t *testing.T, tool, command string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"tool_name": tool, "tool_input": map[string]string{"command": command},
	})
	require.NoError(t, err)
	return string(data)
}

func TestAcceptance(t *testing.T) {
	t.Parallel()
	for _, tc := range commandCases() {
		t.Run(tc.ac+"/"+tc.skill+"/"+tc.command, func(t *testing.T) {
			t.Parallel()
			input := payload(t, "Bash", tc.command)
			got := execute(input, tc.skill)
			assert.Empty(t, got.stdout)
			if !tc.blocked {
				assert.Equal(t, result{}, got)
				return
			}
			assert.Equal(t, 2, got.code)
			header := "Blocked by the " + tc.skill + " skill guard.\n\n"
			assert.True(t, strings.HasPrefix(got.stderr, header))
			assert.NotEmpty(t, strings.TrimSpace(strings.TrimPrefix(got.stderr, header)))
			assert.True(t, strings.HasSuffix(got.stderr, "\n"))
			assert.Contains(t, got.stderr, tc.entry)
			if tc.skill == "pr" {
				assert.NotContains(t, got.stderr, "squash-merge.sh")
			}
		})
	}
}

func TestAC3Parity(t *testing.T) {
	t.Parallel()
	for _, tc := range commandCases() {
		input := payload(t, "Bash", tc.command)
		pr, merge := execute(input, "pr"), execute(input, "merge-pr")
		assert.Equal(t, pr.code, merge.code, tc.command)
		if merge.code != 0 {
			assert.Contains(t, merge.stderr, "squash-merge.sh")
			assert.Contains(t, pr.stderr, "create-pr.sh")
			assert.NotContains(t, pr.stderr, "squash-merge.sh")
		}
	}
}

func TestAC8Payloads(t *testing.T) {
	t.Parallel()
	for _, skill := range []string{"pr", "merge-pr", "watch-pr", "fix-pr"} {
		for _, input := range []string{
			"", "{", "null", "not json", "{}", `{"tool_input":{}}`,
			`{"tool_input":{"command":""}}`, `{"tool_input":{"command":null}}`,
			`{"tool_input":{"command":"gh pr merge 1"}}x`,
		} {
			assert.Equal(t, result{}, execute(input, skill), "%s: %s", skill, input)
		}
		command := "gh pr merge 1; gh pr checks --watch"
		bash := execute(payload(t, "Bash", command), skill)
		assert.Equal(t, 2, bash.code)
		for _, tool := range []string{"Write", "", "Other"} {
			assert.Equal(t, bash, execute(payload(t, tool, command), skill))
		}
	}
}

func TestAC10Determinism(t *testing.T) {
	t.Parallel()
	for _, tc := range commandCases() {
		input := payload(t, "Bash", tc.command)
		want := execute(input, tc.skill)
		for range 3 {
			assert.Equal(t, want, execute(input, tc.skill), "%s: %s", tc.skill, tc.command)
		}
	}
}

func TestUsage(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"unknown"}, {"pr", "extra"}} {
		got := execute("", args...)
		assert.Equal(t, 2, got.code)
		assert.Empty(t, got.stdout)
		assert.Contains(t, got.stderr, "usage: guard-workflow")
	}
}
