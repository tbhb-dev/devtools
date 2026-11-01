// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//nolint:paralleltest // Fake commands change the process environment.
package ghpost

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	targetCall = "api repos/owner/repo/issues/7 --jq {pull: (.pull_request != null), labels: [.labels[].name]}\n"
	labelsCall = "api repos/owner/repo/labels --paginate --jq .[].name\n"
	identity   = "api user --jq .login\n"
)

// setupLabels replaces the stubbed GitHub CLI with one that serves a target, the repository's labels, and records writes.
func setupLabels(t *testing.T, pull bool, current string) string {
	t.Helper()
	dir := setup(t)
	for _, key := range []string{"TARGET_STATUS", "TARGET_ERROR", "LABELS_STATUS", "EDIT_STATUS", "EDIT_STDOUT", "EDIT_STDERR"} {
		t.Setenv(key, "")
	}
	t.Setenv("FIXTURE", dir)
	kind := "false"
	if pull {
		kind = "true"
	}
	write(t, filepath.Join(dir, "target"), `{"pull":`+kind+`,"labels":[`+current+`]}`)
	write(t, filepath.Join(dir, "labels"), "change:docs\nchange:fix\nchange:feat\nmerge:hold\n")
	write(t, filepath.Join(dir, "gh"), `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
case "$*" in
	'api user --jq .login') echo "${LOGIN:-agent}"; exit "${IDENTITY_STATUS:-0}" ;;
	'api repos/owner/repo/issues/7 --jq '*)
		[ -z "$TARGET_ERROR" ] || echo "$TARGET_ERROR" >&2
		cat "$FIXTURE/target"; exit "${TARGET_STATUS:-0}" ;;
	'api repos/owner/repo/labels --paginate --jq .[].name')
		cat "$FIXTURE/labels"; exit "${LABELS_STATUS:-0}" ;;
	'issue edit 7 --repo owner/repo'*|'pr edit 7 --repo owner/repo'*)
		printf '%s\n' "$*" >> "$FIXTURE/writes"
		printf '%s' "$EDIT_STDOUT"; printf '%s' "$EDIT_STDERR" >&2; exit "${EDIT_STATUS:-0}" ;;
	*) exit 99 ;;
esac
`)
	return dir
}

// runLabels runs with a stdin that fails, because a label change must never read input.
func runLabels(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(t.Context(), args, brokenIO{}, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestLabelWriteFailureStdout(t *testing.T) {
	dir := setupLabels(t, false, "")
	t.Setenv("EDIT_STATUS", "1")
	t.Setenv("EDIT_STDERR", "gh: Unprocessable Entity (HTTP 422)\n")
	t.Setenv("EDIT_STDOUT", `{"message":"Unprocessable Entity","errors":["Label rejected"]}`)
	audit := filepath.Join(dir, "state", "devtools", "gh-post.log")
	write(t, audit, "existing record\n")
	code, out, errOut := runLabels(t, "issue", "label", "7", "-R", "owner/repo", "--add", "change:fix")
	require.Equal(t, exitEnvironment, code)
	require.Empty(t, out)
	require.Equal(t, "existing record\n", fileText(t, audit))
	require.Equal(t, "gh-post: gh write failed; check GitHub before retrying\ngh: Unprocessable Entity (HTTP 422)\n"+
		`{"message":"Unprocessable Entity","errors":["Label rejected"]}`+"\n", errOut)
}

func TestLabelWriteFailureSecrets(t *testing.T) {
	token := "ghp_" + strings.Repeat("a", 30)
	tests := []struct {
		name   string
		stderr string
		stdout string
	}{
		{"stderr", token + "\n", ""},
		{"stderr across cap", strings.Repeat(" ", maxWriteErrorBytes-10) + token, ""},
		{"stdout across cap", "", `{"message":"` + strings.Repeat(" ", maxWriteErrorBytes-20) + token + `"}`},
		{"message field", "gh: Unprocessable Entity (HTTP 422)\n", `{"message":"` + token + `"}`},
		{"errors field", "gh: Unprocessable Entity (HTTP 422)\n", `{"errors":["` + token + `"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupLabels(t, false, "")
			t.Setenv("EDIT_STATUS", "1")
			t.Setenv("EDIT_STDERR", tt.stderr)
			t.Setenv("EDIT_STDOUT", tt.stdout)
			audit := filepath.Join(dir, "state", "devtools", "gh-post.log")
			write(t, audit, "existing record\n")
			code, out, errOut := runLabels(t, "issue", "label", "7", "-R", "owner/repo", "--add", "change:fix")
			require.Equal(t, exitEnvironment, code)
			require.NotContains(t, out+errOut, token)
			require.NotContains(t, out+errOut, "ghp_")
			require.Contains(t, errOut, "gh-post: secret line 1\n")
			require.Equal(t, "existing record\n", fileText(t, audit))
		})
	}
}

func TestLabelWriteFailureEscapedText(t *testing.T) {
	dir := setupLabels(t, false, "")
	t.Setenv("EDIT_STATUS", "1")
	t.Setenv("EDIT_STDERR", `echo "<x>\n"`)
	t.Setenv("EDIT_STDOUT", `["<x>\n"]`)
	audit := filepath.Join(dir, "state", "devtools", "gh-post.log")
	write(t, audit, "existing record\n")
	code, out, errOut := runLabels(t, "issue", "label", "7", "-R", "owner/repo", "--add", "change:fix")
	require.Equal(t, exitEnvironment, code)
	require.Empty(t, out)
	require.Equal(t, "existing record\n", fileText(t, audit))
	require.Equal(t, "gh-post: gh write failed; check GitHub before retrying\n"+
		`echo "<x>\n"`+"\n"+`["<x>\n"]`+"\n", errOut)
}

func TestLabelWriteFailureBodySpellingsPreserved(t *testing.T) {
	for _, text := range []string{"<x>/😀", `<x>\/😀`, `\u003c\u0078\u003e\u002f\ud83d\ude00`, `\u003C\u0078\u003E\u002F\uD83D\uDE00`, `<x>/\ud83d\ude00`, `\\u003cx\\u003e\\\\n`, "x", "xy", "xyz"} {
		for _, stream := range []string{"stderr", "stdout"} {
			t.Run(stream+"/"+text, func(t *testing.T) {
				dir := setupLabels(t, false, "")
				t.Setenv("EDIT_STATUS", "1")
				t.Setenv("EDIT_"+strings.ToUpper(stream), text)
				audit := filepath.Join(dir, "state", "devtools", "gh-post.log")
				write(t, audit, "existing record\n")
				code, out, errOut := runLabels(t, "issue", "label", "7", "-R", "owner/repo", "--add", "change:fix")
				require.Equal(t, exitEnvironment, code)
				require.Empty(t, out)
				require.Equal(t, "existing record\n", fileText(t, audit))
				require.Equal(t, "gh-post: gh write failed; check GitHub before retrying\n"+text+"\n", errOut)
			})
		}
	}
}

func TestLabelAddRemove(t *testing.T) {
	tests := []struct {
		name  string
		extra []string
		write string
	}{
		{"add", []string{"--add", "change:fix"}, "--add-label change:fix"},
		{"remove", []string{"--remove", "change:docs"}, "--remove-label change:docs"},
		{
			"both",
			[]string{"--remove", "change:docs", "--add", "change:fix", "--add=change:feat"},
			"--add-label change:fix --add-label change:feat --remove-label change:docs",
		},
		{
			"case variants",
			[]string{"--add", "CHANGE:Fix", "--remove", "Change:DOCS"},
			"--add-label change:fix --remove-label change:docs",
		},
	}
	for _, kind := range []string{"issue", "pr"} {
		for _, tt := range tests {
			t.Run(kind+"/"+tt.name, func(t *testing.T) {
				dir := setupLabels(t, kind == "pr", `"change:docs"`)
				args := append([]string{kind, "label", "7", "-R", "owner/repo"}, tt.extra...)
				code, out, errOut := runLabels(t, args...)
				require.Equal(t, 0, code, errOut)
				require.Empty(t, errOut)
				url := "https://github.com/owner/repo/issues/7\n"
				if kind == "pr" {
					url = "https://github.com/owner/repo/pull/7\n"
				}
				require.Equal(t, url, out)
				require.Equal(
					t,
					kind+" edit 7 --repo owner/repo "+tt.write+"\n",
					fileText(t, filepath.Join(dir, "writes")),
				)
				require.Contains(t, fileText(t, filepath.Join(dir, "state", "devtools", "gh-post.log")), kind+" label")
			})
		}
	}
}

func TestLabelAddReadsRepositoryOnlyWhenNeeded(t *testing.T) {
	dir := setupLabels(t, false, `"change:docs"`)
	code, _, errOut := runLabels(t, "issue", "label", "7", "-R", "owner/repo", "--remove", "change:docs")
	require.Equal(t, 0, code, errOut)
	require.Equal(
		t,
		identity+targetCall+"issue edit 7 --repo owner/repo --remove-label change:docs\n",
		fileText(t, filepath.Join(dir, "calls")),
	)
}

func TestLabelRefusedBeforeAnyCall(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{"merge prefix", []string{"--add", "merge:hold"}, "change:"},
		{"refinement prefix", []string{"--remove", "refinement:ready"}, "change:"},
		{"no colon", []string{"--add", "change"}, "change:"},
		{"prefix inside another word", []string{"--add", "exchange:fix"}, "change:"},
		{"comma mixing prefixes", []string{"--add", "change:fix,merge:hold"}, "comma or a newline"},
		{"comma leading with other prefix", []string{"--add", "merge:hold,change:fix"}, "comma or a newline"},
		{"newline", []string{"--add", "change:fix\nmerge:hold"}, "comma or a newline"},
		{"carriage return", []string{"--remove", "change:fix\rmerge:hold"}, "comma or a newline"},
		{"same label in both", []string{"--add", "change:fix", "--remove", "change:fix"}, "both added and removed"},
		{
			"same label in both, other case",
			[]string{"--add", "change:fix", "--remove", "CHANGE:FIX"},
			"both added and removed",
		},
		{"a refused label beside an allowed one", []string{"--add", "change:fix", "--add", "merge:hold"}, "change:"},
		{"no label", nil, "expected"},
		{"body flag", []string{"--add", "change:fix", "--body-file", "-"}, "expected"},
		{"unknown flag", []string{"--add", "change:fix", "--add-label", "change:feat"}, "expected"},
		{"no repository flag", nil, "expected"},
	}
	for _, kind := range []string{"issue", "pr"} {
		for _, tt := range tests {
			t.Run(kind+"/"+tt.name, func(t *testing.T) {
				dir := setupLabels(t, kind == "pr", "")
				args := append([]string{kind, "label", "7", "-R", "owner/repo"}, tt.args...)
				if tt.name == "no repository flag" {
					args = []string{kind, "label", "7", "--add", "change:fix"}
				}
				code, out, errOut := runLabels(t, args...)
				require.Equal(t, 2, code)
				require.Empty(t, out)
				require.Contains(t, errOut, tt.message)
				require.NoFileExists(t, filepath.Join(dir, "calls"))
				require.NoFileExists(t, filepath.Join(dir, "writes"))
			})
		}
	}
}

func TestLabelOperandIsANumber(t *testing.T) {
	dir := setupLabels(t, false, "")
	for _, operand := range []string{"0", "seven", "-7", "7 8"} {
		code, _, _ := runLabels(t, "issue", "label", operand, "-R", "owner/repo", "--add", "change:fix")
		require.Equal(t, 2, code, operand)
	}
	code, _, _ := runLabels(t, "issue", "label", "-R", "owner/repo", "--add", "change:fix")
	require.Equal(t, 2, code)
	require.NoFileExists(t, filepath.Join(dir, "calls"))
}

func TestLabelUnknownLabelIsNotCreated(t *testing.T) {
	dir := setupLabels(t, false, `"change:docs"`)
	// The first label is valid, so a refusal of the second must not leave a half change behind.
	code, out, errOut := runLabels(
		t,
		"issue",
		"label",
		"7",
		"-R",
		"owner/repo",
		"--add",
		"change:fix",
		"--add",
		"change:nonesuch",
		"--remove",
		"change:docs",
	)
	require.Equal(t, 1, code)
	require.Empty(t, out)
	require.Contains(t, errOut, "does not exist in the repository")
	require.NoFileExists(t, filepath.Join(dir, "writes"))
	require.Equal(t, identity+targetCall+labelsCall, fileText(t, filepath.Join(dir, "calls")))
}

func TestLabelNoOps(t *testing.T) {
	for _, kind := range []string{"issue", "pr"} {
		t.Run(kind, func(t *testing.T) {
			dir := setupLabels(t, kind == "pr", `"change:docs"`)
			code, out, errOut := runLabels(
				t,
				kind,
				"label",
				"7",
				"-R",
				"owner/repo",
				"--add",
				"change:docs",
				"--remove",
				"change:fix",
			)
			require.Equal(t, 0, code, errOut)
			require.Empty(t, errOut)
			require.Contains(t, out, "nothing changed")
			require.NoFileExists(t, filepath.Join(dir, "writes"))
			require.Empty(t, fileText(t, filepath.Join(dir, "state", "devtools", "gh-post.log")))
			require.Equal(t, identity+targetCall, fileText(t, filepath.Join(dir, "calls")))
		})
	}
}

func TestLabelNoOpAddsCaseVariantOfPresentLabel(t *testing.T) {
	dir := setupLabels(t, false, `"change:docs"`)
	code, out, errOut := runLabels(
		t,
		"issue",
		"label",
		"7",
		"-R",
		"owner/repo",
		"--add",
		"CHANGE:DOCS",
		"--add",
		"change:docs",
	)
	require.Equal(t, 0, code, errOut)
	require.Contains(t, out, "nothing changed")
	require.NoFileExists(t, filepath.Join(dir, "writes"))
}

func TestLabelTargetErrors(t *testing.T) {
	tests := []struct {
		name    string
		pull    bool
		kind    string
		env     map[string]string
		code    int
		message string
	}{
		{
			"missing target",
			false,
			"issue",
			map[string]string{"TARGET_STATUS": "1", "TARGET_ERROR": "gh: Not Found (HTTP 404)"},
			1,
			"target not found",
		},
		{
			"github failure",
			false,
			"issue",
			map[string]string{"TARGET_STATUS": "1", "TARGET_ERROR": "gh: Server Error (HTTP 502)"},
			2,
			"cannot read target",
		},
		{"pr number given to issue label", true, "issue", nil, 1, "is a pull request"},
		{"issue number given to pr label", false, "pr", nil, 1, "is an issue"},
		{
			"label list failure",
			false,
			"issue",
			map[string]string{"LABELS_STATUS": "1"},
			2,
			"cannot read repository labels",
		},
		{"write failure", false, "issue", map[string]string{"EDIT_STATUS": "1"}, 2, "gh write failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupLabels(t, tt.pull, "")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			code, out, errOut := runLabels(t, tt.kind, "label", "7", "-R", "owner/repo", "--add", "change:fix")
			require.Equal(t, tt.code, code, errOut)
			require.Empty(t, out)
			require.Contains(t, errOut, tt.message)
			if tt.name != "write failure" {
				require.NoFileExists(t, filepath.Join(dir, "writes"))
			}
		})
	}
}

func TestLabelMalformedTarget(t *testing.T) {
	dir := setupLabels(t, false, "")
	write(t, filepath.Join(dir, "target"), `{"labels":[]}`)
	code, _, errOut := runLabels(t, "issue", "label", "7", "-R", "owner/repo", "--add", "change:fix")
	require.Equal(t, 2, code)
	require.Contains(t, errOut, "cannot read target")
	require.NoFileExists(t, filepath.Join(dir, "writes"))
}

func TestLabelIdentity(t *testing.T) {
	for _, kind := range []string{"issue", "pr"} {
		t.Run(kind, func(t *testing.T) {
			dir := setupLabels(t, kind == "pr", "")
			t.Setenv("LOGIN", "stranger")
			code, out, errOut := runLabels(t, kind, "label", "7", "-R", "owner/repo", "--add", "change:fix")
			require.Equal(t, 1, code)
			require.Empty(t, out)
			require.Equal(t, "gh-post: identity refused\n", errOut)
			require.Equal(t, identity, fileText(t, filepath.Join(dir, "calls")))
			require.NoFileExists(t, filepath.Join(dir, "writes"))
		})
	}
}

func TestLabelIdentityFollowsTargetRepository(t *testing.T) {
	dir := setupLabels(t, false, "")
	t.Setenv("DEVTOOLS_POST_IDENTITIES", "")
	write(t, filepath.Join(dir, "config", "devtools", "repo.conf"), "post_identities = other-agent\n")
	code, _, errOut := runLabels(t, "issue", "label", "7", "-R", "owner/repo", "--add", "change:fix")
	require.Equal(t, 1, code)
	require.Equal(t, "gh-post: identity refused\n", errOut)
	require.NoFileExists(t, filepath.Join(dir, "writes"))
}

func TestLabelLabelsAreScanned(t *testing.T) {
	dir := setupLabels(t, false, "")
	code, _, errOut := runLabels(t, "issue", "label", "7", "-R", "owner/repo", "--add", "change:file:///tmp/x")
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "line 1")
	require.NoFileExists(t, filepath.Join(dir, "calls"))
}
