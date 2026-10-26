// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const plannerFixture = "{\"type\":\"PLANNER_RESPONSE\",\"input_tokens\":0,\"output_tokens\":23,\"cache_read_tokens\":7,\"text\":\"EXCLUDED_TRANSCRIPT\"}\n"

func TestTranscriptTokens(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, data, wantError string }{
		{"small", strings.Repeat(" \n", 5000) + plannerFixture, ""},
		{"exact", strings.Repeat(" ", transcriptWindow-len(plannerFixture)-1) + "\n" + plannerFixture, ""},
		{"large", strings.Repeat(" ", 1024*1024-len(plannerFixture)-1) + "\n" + plannerFixture, ""},
		{"cut line", strings.Repeat("x", transcriptWindow) + "\n" + plannerFixture, ""},
		{"cut response", plannerFixture[:len(plannerFixture)-1] + strings.Repeat(" ", transcriptWindow) + "\n", "window"},
		{"no response", "{}\n", "window"},
		{"incomplete", strings.TrimSuffix(plannerFixture, "\n"), "window"},
		{"last complete", plannerFixture + "{\"type\":\"PLANNER_RESPONSE\"}", ""},
		{"last response invalid", plannerFixture + "{\"type\":\"PLANNER_RESPONSE\"}\n", "format"},
		{"negative", strings.Replace(plannerFixture, ":0", ":-1", 1), "format"},
		{"string", strings.Replace(plannerFixture, ":0", ":\"1\"", 1), "format"},
		{"fraction", strings.Replace(plannerFixture, ":0", ":1.5", 1), "format"},
		{"missing", "", "missing"},
		{"unreadable", "", "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			name := filepath.Join(t.TempDir(), "EXCLUDED_PATH")
			if tc.name == "unreadable" {
				require.NoError(t, os.Mkdir(name, 0o700))
			} else if tc.name != "missing" {
				require.NoError(t, os.WriteFile(name, []byte(tc.data), 0o600))
			}
			input, err := json.Marshal(
				map[string]any{"transcriptPath": name, "modelName": "fixture-model", "error": "EXCLUDED_ERROR"},
			)
			require.NoError(t, err)
			state := t.TempDir()
			runHook([]string{"hook", "PostInvocation"}, bytes.NewReader(input), state, realOpen)
			got := readRecord(t, state)
			if tc.wantError != "" {
				assert.Equal(t, map[string]any{"error": tc.wantError}, got["tokens"])
			} else {
				assert.Equal(
					t,
					map[string]any{"in": float64(0), "out": float64(23), "cache_read": float64(7)},
					got["tokens"],
				)
			}
			assert.Equal(t, "fixture-model", got["model"])
			assert.Equal(t, true, got["error"])
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "EXCLUDED")
		})
	}
}

func TestCommandAttempts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		line string
		want []string
	}{
		{"git commit -m x", []string{"commit"}},
		{"git -C dir commit", []string{"commit"}},
		{"git -c x=y -C 'some dir' commit", []string{"commit"}},
		{"cd x && git commit", []string{"commit"}},
		{"bash path/to/commit.sh", []string{"commit"}},
		{"sh 'path to/commit.sh'", []string{"commit"}},
		{"gh pr create", []string{"pr_publish"}},
		{"gh-post pr create", []string{"pr_publish"}},
		{"path/create-pr.sh", []string{"pr_publish"}},
		{"bash x/publish.sh", []string{"pr_publish"}},
		{"git commit -m x && gh-post pr create", []string{"commit", "pr_publish"}},
		{"git commit -m a; git commit -m b", []string{"commit"}},
		{"false || git commit | gh pr create\ngit commit", []string{"commit", "pr_publish"}},
		{"git commit --dry-run", nil},
		{"bash x/commit.sh --dry-run", nil},
		{"bash x/create-pr.sh --dry-run", nil},
		{"bash x/create-pr.sh --check-description", nil},
		{"x/publish.sh --check-description", nil},
		{"git commit-tree", nil},
		{"git log --grep commit", nil},
		{"git --version commit", nil},
		{"git -c invalid commit", nil},
		{"git -C", nil},
		{"git commit -m \"escaped \\\"quote\"", []string{"commit"}},
		{"echo gh pr create", nil},
		{"echo 'git commit'", nil},
		{"echo \"x && git commit\"", nil},
		{"gh pr view", nil},
		{"ls commit.sh.bak", nil},
		{"git commit -m 'unbalanced", nil},
		{"git commit --dry-run; gh pr create", []string{"pr_publish"}},
	} {
		t.Run(tc.line, func(t *testing.T) { t.Parallel(); assert.Equal(t, tc.want, commandAttempts(tc.line)) })
	}
}

func TestDerivationIsolation(t *testing.T) {
	t.Parallel()
	binary := buildHook(t)
	for _, event := range []string{"PreToolUse", "PostToolUse", "PostInvocation"} {
		for _, line := range []string{"git commit -m EXCLUDED_COMMAND", "bash x/create-pr.sh", "git commit -m 'EXCLUDED_UNBALANCED"} {
			input, err := json.Marshal(
				map[string]any{
					"modelName":      "fixture-model",
					"transcriptPath": "EXCLUDED_MISSING",
					"error":          "EXCLUDED_ERROR",
					"toolCall": map[string]any{
						"name": "run_command",
						"args": map[string]string{"CommandLine": line},
					},
				},
			)
			require.NoError(t, err)
			state := t.TempDir()
			output, err := invoke(t.Context(), binary, state, input, "hook", event)
			require.NoError(t, err)
			require.Empty(t, output)
			got := readRecord(t, state)
			assert.Equal(t, "fixture-model", got["model"])
			assert.Equal(t, true, got["error"])
			if event == "PostToolUse" && !strings.Contains(line, "UNBALANCED") {
				assert.Contains(t, got, "vcs_attempts")
			} else {
				assert.NotContains(t, got, "vcs_attempts")
			}
			if event == "PostInvocation" {
				assert.Equal(t, map[string]any{"error": "missing"}, got["tokens"])
			} else {
				assert.NotContains(t, got, "tokens")
			}
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "EXCLUDED")
		}
	}
}
