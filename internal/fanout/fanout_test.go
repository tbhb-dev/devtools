// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package fanout_test

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/fanout"
)

func TestLedger(t *testing.T) {
	t.Parallel()
	entries := []fanout.Entry{
		{Repository: "org/active", Disposition: "active"},
		{Repository: "org/inactive", Disposition: "inactive"},
		{Repository: "org/exempt", Disposition: "exempt"},
	}
	data := ledger(t, entries)
	got, err := fanout.ValidateLedger(data)
	require.NoError(t, err)
	require.Equal(t, entries, got)
	oversized := make([]fanout.Entry, 101)
	for i := range oversized {
		oversized[i] = fanout.Entry{Repository: fmt.Sprintf("org/repo%d", i), Disposition: "active"}
	}
	boundary, err := fanout.ValidateLedger(ledger(t, oversized[:100]))
	require.NoError(t, err)
	require.Len(t, boundary, 100)
	for name, input := range map[string][]byte{
		"oversized":              ledger(t, oversized),
		"duplicate":              ledger(t, append(entries, entries[0])),
		"case duplicate":         ledger(t, append(entries, fanout.Entry{Repository: "ORG/ACTIVE", Disposition: "active"})),
		"unknown version":        []byte(`{"version":2,"entries":[]}`),
		"malformed":              []byte(`secret-password`),
		"unknown field":          []byte(`{"version":1,"entries":[],"secret-password":true}`),
		"bad repository":         ledger(t, []fanout.Entry{{Repository: "https://secret-password@host/repo", Disposition: "active"}}),
		"bad disposition":        ledger(t, []fanout.Entry{{Repository: "org/repo", Disposition: "secret-password"}}),
		"null":                   []byte(`null`),
		"duplicate version":      []byte(`{"version":2,"version":1,"entries":[]}`),
		"duplicate entries":      []byte(`{"version":1,"entries":[{"repository":"secret-password"}],"entries":[]}`),
		"duplicate repository":   []byte(`{"version":1,"entries":[{"repository":"secret-password","repository":"org/repo","disposition":"active"}]}`),
		"duplicate disposition":  []byte(`{"version":1,"entries":[{"repository":"org/repo","disposition":"active","disposition":"exempt"}]}`),
		"escaped duplicate":      []byte(`{"version":1,"entries":[{"repository":"org/repo","disposition":"active","\u0064\u0069\u0073\u0070\u006f\u0073\u0069\u0074\u0069\u006f\u006e":"exempt"}]}`),
		"case duplicate member":  []byte(`{"version":1,"entries":[{"repository":"org/repo","disposition":"active","Disposition":"exempt"}]}`),
		"unicode case duplicate": []byte(`{"version":1,"entries":[{"repository":"org/repo","disposition":"active","\u0064\u0069\u0073\u0070\u006f\u017f\u0069\u0074\u0069\u006f\u006e":"exempt"}]}`),
		"null version":           []byte(`{"version":null,"entries":[]}`),
		"null after version":     []byte(`{"version":1,"version":null,"entries":[]}`),
		"null repository":        []byte(`{"version":1,"entries":[{"repository":null,"disposition":"active"}]}`),
		"null after repository":  []byte(`{"version":1,"entries":[{"repository":"org/repo","repository":null,"disposition":"active"}]}`),
		"null disposition":       []byte(`{"version":1,"entries":[{"repository":"org/repo","disposition":null}]}`),
		"null after disposition": []byte(`{"version":1,"entries":[{"repository":"org/repo","disposition":"active","disposition":null}]}`),
		"nil entries":            []byte(`{"version":1,"entries":null}`),
		"nil entry":              []byte(`{"version":1,"entries":[null]}`),
		"missing entries":        []byte(`{"version":1}`),
		"trailing":               append(data, []byte(`{}`)...),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			refused, refusal := fanout.ValidateLedger(input)
			require.Equal(t, fanout.ErrLedger, refusal)
			require.Nil(t, refused)
			require.NotContains(t, refusal.Error(), "secret-password")
		})
	}
}

func ledger(t *testing.T, entries []fanout.Entry) []byte {
	t.Helper()
	data, err := json.Marshal(struct {
		Version int            `json:"version"`
		Entries []fanout.Entry `json:"entries"`
	}{Version: 1, Entries: entries})
	require.NoError(t, err)
	return data
}

func TestDecisions(t *testing.T) {
	t.Parallel()
	tagged := time.Unix(1700000000, 0)
	for _, tc := range []struct {
		offset          time.Duration
		allowed, missed bool
		deadline        time.Duration
	}{
		{54 * time.Minute, true, false, time.Hour},
		{55 * time.Minute, false, false, time.Hour},
		{56 * time.Minute, false, false, time.Hour},
		{time.Hour - time.Nanosecond, false, false, time.Hour},
		{time.Hour, false, true, 65 * time.Minute},
		{61 * time.Minute, false, true, 66 * time.Minute},
	} {
		t.Run(tc.offset.String(), func(t *testing.T) {
			t.Parallel()
			got := fanout.Decide(tagged, tagged.Add(tc.offset))
			require.Equal(t, tc.allowed, got.WriteAllowed)
			require.Equal(t, tc.missed, got.MissedObjective)
			require.Equal(t, tagged.Add(tc.deadline), got.ReportingDeadline)
			if !tc.allowed {
				require.Equal(t, "late-event", got.Result)
			}
		})
	}
}

func TestAggregation(t *testing.T) {
	t.Parallel()
	entries := []fanout.Entry{
		{Repository: "org/a", Disposition: "active"},
		{Repository: "org/i", Disposition: "inactive"},
		{Repository: "org/e", Disposition: "exempt"},
	}
	results := []fanout.Result{
		{Repository: "org/a", Value: "current"},
		{Repository: "org/i", Value: "inactive"},
		{Repository: "org/e", Value: "exempt"},
	}
	for value := range strings.FieldsSeq("current existing opened newer late-event deadline prepare-failed publish-failed access-refused scan-refused conflict uncertain") {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			own := append([]fanout.Result(nil), results...)
			own[0].Value = value
			got, err := fanout.Summarize(entries, own)
			require.NoError(t, err)
			require.Equal(t, 1, got.Active)
			if value == "current" || value == "existing" || value == "opened" || value == "newer" {
				require.Equal(t, fanout.Aggregate{Active: 1, Adopted: 1, Outcome: "passed"}, got)
			} else {
				require.Equal(t, fanout.Aggregate{Active: 1, Outcome: "failed"}, got)
			}
		})
	}
	for name, invalid := range map[string][]fanout.Result{
		"missing":            results[1:],
		"extra":              append(append([]fanout.Result(nil), results...), fanout.Result{Repository: "org/extra", Value: "current"}),
		"duplicate":          {results[0], results[0], results[2]},
		"unknown repository": {{Repository: "org/other", Value: "current"}, results[1], results[2]},
		"unknown value":      {{Repository: "org/a", Value: "secret-password"}, results[1], results[2]},
		"inactive active":    {{Repository: "org/a", Value: "inactive"}, results[1], results[2]},
		"adopted inactive":   {results[0], {Repository: "org/i", Value: "current"}, results[2]},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := fanout.Summarize(entries, invalid)
			require.Error(t, err)
			require.Equal(t, fanout.Aggregate{}, got)
			require.NotContains(t, err.Error(), "secret-password")
		})
	}
	got, err := fanout.Summarize(entries[1:], results[1:])
	require.NoError(t, err)
	require.Equal(t, fanout.Aggregate{Outcome: "no-active-entries"}, got)
	got, err = fanout.Summarize(nil, nil)
	require.NoError(t, err)
	require.Equal(t, "no-active-entries", got.Outcome)
}

func TestRelease(t *testing.T) {
	// PATH isolates external reads, so these fixtures must run sequentially.
	root := t.TempDir()
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	run := func(name string, args ...string) string {
		t.Helper()
		command := exec.CommandContext(t.Context(), name, args...)
		command.Dir = root
		out, runErr := command.CombinedOutput()
		require.NoError(t, runErr, "%s", out)
		return strings.TrimSpace(string(out))
	}
	run(realGit, "init", "-q", "--initial-branch=main")
	run(realGit, "config", "user.name", "tbhb.dev Agent")
	run(realGit, "config", "user.email", "334376894+tbhb-agent@users.noreply.github.com")
	run(realGit, "config", "gpg.format", "ssh")
	key := filepath.Join(root, "signing")
	run("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key)
	run(realGit, "config", "user.signingkey", key)
	write := func(path, body string, mode os.FileMode) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o750))
		directory, openErr := os.OpenRoot(root)
		require.NoError(t, openErr)
		defer directory.Close()
		require.NoError(t, directory.WriteFile(path, []byte(body), mode))
	}
	original := []fanout.Entry{{Repository: "org/release", Disposition: "active"}}
	write(fanout.LedgerPath, string(ledger(t, original)), 0o600)
	run(realGit, "add", "--", fanout.LedgerPath)
	t.Setenv("GIT_AUTHOR_NAME", "tbhb-dev-releases[bot]")
	t.Setenv("GIT_AUTHOR_EMAIL", "340658377+tbhb-dev-releases[bot]@users.noreply.github.com")
	run(realGit, "-c", "commit.gpgsign=true", "commit", "-qm", "release")
	commit := run(realGit, "rev-parse", "HEAD")
	t.Setenv("GIT_COMMITTER_DATE", "2026-01-02T03:04:05Z")
	run(realGit, "tag", "-s", "v1.2.3", "-m", "release")
	run(realGit, "-c", "tag.gpgsign=false", "-c", "tag.forceSignAnnotated=false", "tag", "v1.2.4")
	run(realGit, "-c", "tag.gpgsign=false", "tag", "-a", "v1.2.5", "-m", "unsigned")
	origin := filepath.Join(t.TempDir(), "origin.git")
	run(realGit, "init", "--bare", "-q", origin)
	run(realGit, "remote", "add", "origin", origin)
	run(realGit, "push", "-q", "origin", "main", "--tags")
	run(realGit, "fetch", "-q", "origin")
	verifier, err := os.ReadFile("../../tools/release-verify.sh")
	require.NoError(t, err)
	write("tools/release-verify.sh", string(verifier), 0o700)
	write("tools/check-versions.sh", "#!/bin/sh\nexit 0\n", 0o700)
	write(fanout.LedgerPath, `{"version":2,"entries":[]}`, 0o600)
	// Record every external operation, including Git transport to the local origin.
	write("bin/git", `#!/bin/sh
printf 'git %s\n' "$*" >> "$CALLS"
exec "$REAL_GIT" "$@"
`, 0o700)
	write("bin/gh", `#!/bin/sh
printf 'gh %s\n' "$*" >> "$CALLS"
[ "$#" -eq 4 ] && [ "$1" = api ] && [ "$3" = --jq ] || exit 99
case "$*" in
*commits*) printf '%s\n' "$COMMIT_VERDICT";;
*) echo 'true valid';;
esac
`, 0o700)
	calls := filepath.Join(root, "calls")
	t.Setenv("PATH", filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CALLS", calls)
	t.Setenv("REAL_GIT", realGit)
	t.Setenv("COMMIT_VERDICT", "true valid")
	got, err := fanout.Load(t.Context(), root, "v1.2.3")
	require.NoError(t, err)
	require.Equal(t, original, got.Entries)
	require.Equal(t, commit, got.Commit)
	require.Equal(t, time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC), got.TaggedAt)
	for _, tag := range []string{"v1.2.4", "v1.2.5"} {
		refused, loadErr := fanout.Load(t.Context(), root, tag)
		require.Error(t, loadErr)
		require.Equal(t, fanout.Release{}, refused)
	}
	t.Setenv("COMMIT_VERDICT", "false secret-password")
	refused, err := fanout.Load(t.Context(), root, "v1.2.3")
	require.Error(t, err)
	require.Equal(t, fanout.Release{}, refused)
	require.NotContains(t, err.Error(), "secret-password")
	recorded, err := os.ReadFile(calls)
	require.NoError(t, err)
	for line := range strings.SplitSeq(strings.TrimSpace(string(recorded)), "\n") {
		fields := strings.Fields(line)
		require.GreaterOrEqual(t, len(fields), 2)
		if fields[0] == "git" {
			require.Contains(
				t,
				[]string{"check-ref-format", "rev-parse", "cat-file", "describe", "ls-remote", "merge-base", "show"},
				fields[1],
			)
		} else {
			require.True(t, strings.HasPrefix(line, "gh api repos/{owner}/{repo}/git/tags/") ||
				strings.HasPrefix(line, "gh api repos/{owner}/{repo}/commits/"), "%s", line)
			require.NotContains(t, line, "--method")
			require.NotContains(t, line, "--field")
		}
	}
}

func TestNoScheduling(t *testing.T) {
	t.Parallel()
	// Inspect the package to catch scheduling through a helper as well.
	source, err := parser.ParseFile(token.NewFileSet(), "fanout.go", nil, 0)
	require.NoError(t, err)
	for _, dependency := range source.Imports {
		require.Contains(t, []string{
			`"bytes"`, `"context"`, `"encoding/json"`, `"errors"`, `"io"`,
			`"os/exec"`, `"regexp"`, `"strconv"`, `"strings"`, `"time"`,
		}, dependency.Path.Value)
	}
	ast.Inspect(source, func(node ast.Node) bool {
		if loop, isLoop := node.(*ast.ForStmt); isLoop {
			// Decoder.More loops consume a finite snapshot rather than poll.
			call, ok := loop.Cond.(*ast.CallExpr)
			require.True(t, ok, "polling loop")
			selector, ok := call.Fun.(*ast.SelectorExpr)
			require.True(t, ok, "polling loop")
			receiver, ok := selector.X.(*ast.Ident)
			require.True(t, ok, "polling loop")
			require.Equal(t, "decoder", receiver.Name)
			require.Equal(t, "More", selector.Sel.Name)
		}
		_, background := node.(*ast.GoStmt)
		require.False(t, background, "background goroutine")
		call, ok := node.(*ast.CallExpr)
		if ok {
			if selector, selected := call.Fun.(*ast.SelectorExpr); selected {
				require.NotContains(
					t,
					[]string{"Sleep", "After", "AfterFunc", "Tick", "NewTimer", "NewTicker"},
					selector.Sel.Name,
				)
			}
		}
		return true
	})
}
