// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

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

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestRedesign(t *testing.T) {
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const record = "subject tree:example\nbound 2\nfinding " + digest + "\nfinding " + digest + "\n"
	for _, kind := range []string{"commit", "pr", "squash"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			git := exec.CommandContext(t.Context(), "git", "init", "--quiet")
			require.NoError(t, git.Run())
			require.NoError(t, os.MkdirAll(".agents", 0o700))
			require.NoError(t, os.WriteFile(".agents/agent-skills.conf", []byte("review_max_rounds = 2\n"), 0o600))
			state := filepath.Join(".git", "agent-skills")
			require.NoError(t, os.MkdirAll(state, 0o700))
			file := filepath.Join(state, kind+".rounds")
			stamp := filepath.Join(state, kind+".stamp")
			require.NoError(t, os.WriteFile(stamp, []byte("preserve"), 0o600))
			invoke := func(reason string) (int, string) {
				var out bytes.Buffer
				code := agentrepo.Run([]string{"review", "redesign", kind, "--reason", reason}, nil, &out, &out)
				return code, out.String()
			}
			code, output := invoke("Rewrite the rationale")
			require.Equal(t, 3, code, output)
			require.Contains(t, output, "no active")
			require.NoFileExists(t, file)
			for _, tc := range []struct{ name, data, why string }{
				{"AC1 below bound", "subject tree:example\nbound 2\nfinding " + digest + "\n", "not at"},
				{"AC7 legacy", "subject tree:example\nrounds 2\n", "legacy"},
				{"AC7 count only", "2\n", "legacy"},
				{"AC1 malformed", "subject tree:example\nbound 2\nfinding unknown\n", "inspect"},
				{"AC5 second start", record + "redesign {}\nfinding " + digest + "\nfinding " + digest + "\n", "inspection"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					require.NoError(t, os.WriteFile(file, []byte(tc.data), 0o600))
					code, output = invoke("Rewrite the rationale")
					require.Equal(t, 3, code, output)
					require.Contains(t, output, tc.why)
					data, err := os.ReadFile(file)
					require.NoError(t, err)
					require.Equal(t, tc.data, string(data))
				})
			}
			require.NoError(t, os.WriteFile(file, []byte(record), 0o600))
			code, output = invoke(" ")
			require.NotZero(t, code, output)
			before := time.Now().UTC().Add(-time.Second)
			code, output = invoke("Rewrite the rationale\nfrom scratch")
			require.Zero(t, code, output)
			data, err := os.ReadFile(file)
			require.NoError(t, err)
			require.True(t, strings.HasPrefix(string(data), record), "AC2 history must be append only")
			eventLine := strings.TrimSuffix(strings.TrimPrefix(string(data), record), "\n")
			require.True(t, strings.HasPrefix(eventLine, "redesign "))
			var event struct {
				Kind     string   `json:"kind"`
				Subject  string   `json:"subject"`
				At       string   `json:"at"`
				Reason   string   `json:"reason"`
				Bound    int      `json:"bound"`
				Rejected []string `json:"rejected"`
			}
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(eventLine, "redesign ")), &event))
			require.Equal(t, kind, event.Kind)
			require.Equal(t, "tree:example", event.Subject)
			require.Equal(t, "Rewrite the rationale\nfrom scratch", event.Reason)
			require.Equal(t, 2, event.Bound)
			require.Equal(t, []string{digest, digest}, event.Rejected)
			at, err := time.Parse(time.RFC3339, event.At)
			require.NoError(t, err)
			require.True(t, !at.Before(before) && !at.After(time.Now().UTC()))
			require.True(t, strings.HasSuffix(event.At, "Z"))
			code, output = invoke("Try again")
			require.Equal(t, 3, code, output)
			require.Contains(t, output, "inspection")
			preserved, err := os.ReadFile(stamp)
			require.NoError(t, err)
			require.Equal(t, "preserve", string(preserved), "AC6 authority and stamps stay unchanged")
		})
	}
}

func TestRedesignGuidance(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"commit", "pr", "merge-pr", "agent-commit", "agent-pr", "agent-merge-pr"} {
		for _, harness := range []string{"claude", "codex"} {
			data, err := os.ReadFile(
				filepath.Join("..", "..", "packages", "agents", harness, ".apm", "skills", name, "SKILL.md"),
			)
			require.NoError(t, err)
			require.Contains(t, string(data), "agentrepo review redesign", "AC8 "+name)
			require.Contains(t, string(data), "rejected digest", "AC8 "+name)
			require.Contains(t, string(data), "one redesign", "AC8 "+name)
		}
	}
}
