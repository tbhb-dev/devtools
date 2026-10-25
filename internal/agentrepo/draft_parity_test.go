// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func draftProcess(tb testing.TB, command string, args ...string) draftResult {
	tb.Helper()
	cmd := exec.CommandContext(tb.Context(), command, args...)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err := cmd.Run()
	if err != nil {
		require.NotNil(tb, cmd.ProcessState, "%v", err)
	}
	return draftResult{out.String(), stderr.String(), cmd.ProcessState.ExitCode()}
}

func TestLintDraftParity(t *testing.T) {
	t.Setenv("LC_ALL", "C.UTF-8")
	f := newDraftFixture(t, true)
	for _, tc := range []struct {
		name string
		args []string
		env  map[string]string
	}{
		{name: "clean", args: []string{"draft.md"}},
		{name: "absolute inside", args: []string{filepath.Join(f.root, "draft.md")}},
		{name: "non Markdown", args: []string{"draft.txt"}},
		{name: "vale finding", args: []string{"draft.md"}, env: map[string]string{"DRAFT_VALE_CODE": "1", "DRAFT_VALE_OUT": "vale finding\n", "DRAFT_VALE_ERR": "vale diagnostic\n"}},
		{name: "cspell finding", args: []string{"draft.md"}, env: map[string]string{"DRAFT_CSPELL_CODE": "1", "DRAFT_CSPELL_OUT": "cspell finding\n"}},
		{name: "rumdl finding", args: []string{"draft.md"}, env: map[string]string{"DRAFT_RUMDL_CODE": "1", "DRAFT_RUMDL_ERR": "rumdl finding\n"}},
		{name: "all gates fail", args: []string{"draft.md"}, env: map[string]string{"DRAFT_VALE_CODE": "1", "DRAFT_CSPELL_CODE": "1", "DRAFT_RUMDL_CODE": "1", "DRAFT_VALE_OUT": "vale\n", "DRAFT_CSPELL_OUT": "cspell\n", "DRAFT_RUMDL_OUT": "rumdl\n"}},
		{name: "unscoped", args: []string{"draft.md"}, env: map[string]string{"DRAFT_PROBE_OUT": ""}},
		{name: "unscoped one", args: []string{"draft.md"}, env: map[string]string{"DRAFT_PROBE_OUT": "noise\n", "DRAFT_PROBE_CODE": "1"}},
		{name: "failed vale", args: []string{"draft.md"}, env: map[string]string{"DRAFT_PROBE_OUT": "", "DRAFT_PROBE_ERR": "failure\ndetail\n\n", "DRAFT_PROBE_CODE": "2"}},
		{name: "no argument"},
		{name: "empty argument", args: []string{""}},
		{name: "missing", args: []string{"absent.md"}},
		{name: "outside absolute", args: []string{f.outside}},
		{name: "outside parent", args: []string{"../outside.md"}},
		{name: "outside symlink", args: []string{"linked.md"}},
		{name: "outside symlink parent", args: []string{"escape/../draft.md"}},
		{name: "Go", args: []string{"draft.go"}},
		{name: "Python", args: []string{"draft.py"}},
		{name: "shell", args: []string{"draft.sh"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			old := draftProcess(t, "bash", append([]string{f.legacy}, tc.args...)...)
			require.Equal(t, old, draftProcess(t, f.binary, append([]string{"lint-draft"}, tc.args...)...))
			require.Equal(t, old, draftProcess(t, "bash", append([]string{f.shim}, tc.args...)...))
		})
	}
}

func TestLintDraftProcessBudget(t *testing.T) {
	t.Setenv("LC_ALL", "C.UTF-8")
	f := newDraftFixture(t, true)
	require.NoError(t, os.WriteFile("draft.md", []byte(strings.Repeat("A draft.\n", 500)), 0o600))
	start := time.Now()
	got := draftProcess(t, f.binary, "lint-draft", "draft.md")
	elapsed := time.Since(start)
	require.Equal(t, draftResult{}, got)
	require.Less(t, elapsed, draftBudget)
	t.Logf("process overhead: %s (budget %s)", elapsed, draftBudget)
}

func BenchmarkLintDraft(b *testing.B) {
	f := newDraftFixture(b, true)
	require.NoError(b, os.WriteFile("draft.md", []byte(strings.Repeat("A draft.\n", 500)), 0o600))
	b.ResetTimer()
	for b.Loop() {
		require.Equal(b, draftResult{}, draftProcess(b, f.binary, "lint-draft", "draft.md"))
	}
}
