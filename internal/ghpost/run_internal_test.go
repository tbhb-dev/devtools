// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//nolint:paralleltest // Fake commands and configuration tests change the process environment.
package ghpost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func setup(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake gh uses a POSIX shell")
	}
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	for _, key := range []string{"DEVTOOLS_PRIVATE_STRINGS", "DEVTOOLS_PRIVATE_STRINGS_REF", "DEVTOOLS_PRIVATE_STRINGS_TOKEN_COMMAND", "DEVTOOLS_SCAN_MODE", "DEVTOOLS_POST_IDENTITIES", "GH_TOKEN", "OP_SERVICE_ACCOUNT_TOKEN", "LOGIN", "IDENTITY_STATUS", "GRAPHQL_STATUS", "WRITE_STATUS", "SILENT", "PRIVATE_RESULT", "HEAD_OID", "HEAD_STATUS"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
	t.Setenv("DEVTOOLS_POST_IDENTITIES", "agent,other-agent")
	t.Setenv("CALLS", filepath.Join(dir, "calls"))
	t.Setenv("PAYLOAD", filepath.Join(dir, "payload"))
	write(t, filepath.Join(dir, "gh"), `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
case "$*" in
	'api user --jq .login') echo "${LOGIN:-agent}"; exit "${IDENTITY_STATUS:-0}" ;;
	'api graphql '*) echo 'app[bot]'; exit "${GRAPHQL_STATUS:-1}" ;;
	*'--json headRefOid'*) echo "${HEAD_OID:-0123456789abcdef0123456789abcdef01234567}"; exit "${HEAD_STATUS:-0}" ;;
	*' view '*) echo 'https://github.com/owner/repo/pull/12'; exit 0 ;;
esac
cat > "$PAYLOAD"
if [ "${WRITE_STATUS:-0}" != 0 ]; then cat "$PAYLOAD" >&2; exit 1; fi
if [ "${SILENT:-0}" != 1 ]; then echo 'https://github.com/owner/repo/pull/12'; fi
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func write(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	//nolint:gosec // Fake commands must be executable to test the process boundary.
	require.NoError(t, os.Chmod(path, 0o700))
}

func run(t *testing.T, body string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(t.Context(), args, strings.NewReader(body), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestPostAndAudit(t *testing.T) {
	dir := setup(t)
	body := "Public text\n"
	bodyFile := filepath.Join(dir, "body")
	write(t, bodyFile, body)
	for range 2 {
		code, out, errOut := run(t, "unused", "pr", "comment", "12", "-R", "owner/repo", "--body-file", bodyFile)
		require.Equal(t, 0, code, errOut)
		require.Equal(t, "https://github.com/owner/repo/pull/12\n", out)
		require.Empty(t, errOut)
	}
	payload, err := os.ReadFile(filepath.Join(dir, "payload"))
	require.NoError(t, err)
	require.Equal(t, body, string(payload))
	data, err := os.ReadFile(filepath.Join(dir, "state", "devtools", "gh-post.log"))
	require.NoError(t, err)
	records := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, records, 2)
	for _, line := range records {
		var record auditRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		require.Equal(t, "pr comment", record.Operation)
		require.Equal(t, "https://github.com/owner/repo/pull/12", record.URL)
		hash := sha256.Sum256([]byte(body))
		require.Equal(t, hex.EncodeToString(hash[:]), record.SHA256)
		_, err = time.Parse(time.RFC3339Nano, record.Time)
		require.NoError(t, err)
		require.NotContains(t, line, body)
	}
}

func TestPostWithGHConfigDir(t *testing.T) {
	for _, tt := range []struct {
		name, login string
		graphql     bool
	}{
		{"app", "app[bot]", true},
		{"user", "agent", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := setup(t)
			configDir := filepath.Join(dir, "gh-config")
			write(
				t,
				filepath.Join(configDir, "hosts.yml"),
				"github.com:\n    user: "+tt.login+"\n    oauth_token: test-placeholder\n    git_protocol: https\n",
			)
			t.Setenv("GH_CONFIG_DIR", configDir)
			write(t, filepath.Join(dir, "gh"), `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
login=$(sed -n 's/^    user: //p' "$GH_CONFIG_DIR/hosts.yml")
case "$*" in
	'api user --jq .login')
		[ "$login" != 'app[bot]' ] || exit 1
		echo "$login"; exit 0 ;;
	'api graphql '*) echo "$login"; exit 0 ;;
esac
cat > "$PAYLOAD"
echo 'https://github.com/owner/repo/pull/12'
`)
			t.Setenv("DEVTOOLS_POST_IDENTITIES", tt.login)
			code, out, errOut := run(t, "Public text\n", "pr", "comment", "12", "-R", "owner/repo")
			require.Equal(t, 0, code, errOut)
			require.Equal(t, "https://github.com/owner/repo/pull/12\n", out)
			require.Empty(t, errOut)
			calls, err := os.ReadFile(filepath.Join(dir, "calls"))
			require.NoError(t, err)
			require.Equal(t, tt.graphql, strings.Contains(string(calls), "api graphql"))
			payload, err := os.ReadFile(filepath.Join(dir, "payload"))
			require.NoError(t, err)
			require.Equal(t, "Public text\n", string(payload))
		})
	}
}

func TestReviewEdit(t *testing.T) {
	for _, tt := range []struct {
		name, source, body, login string
		code                      int
	}{
		{"stdin", "", "Public correction\n", "agent", 0},
		{"explicit stdin", "-", "Public correction\n", "agent", 0},
		{"file", "file", "Public correction\n\"quoted\"", "agent", 0},
		{"scan refused", "file", "file:" + "///tmp/report", "agent", 1},
		{"identity refused", "file", "Public correction\n", "person", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := setup(t)
			t.Setenv("LOGIN", tt.login)
			args := []string{"review", "edit", "12", "42", "-R", "owner/repo"}
			stdin := tt.body
			if tt.source != "" {
				path := tt.source
				if path != "-" {
					path = filepath.Join(dir, "body")
					write(t, path, tt.body)
					stdin = "unused"
				}
				args = append(args, "--body-file", path)
			}
			code, out, errOut := run(t, stdin, args...)
			require.Equal(t, tt.code, code, errOut)
			auditPath := filepath.Join(dir, "state", "devtools", "gh-post.log")
			if tt.code != 0 {
				require.Empty(t, out)
				require.NotEmpty(t, errOut)
				require.NotContains(t, errOut, tt.body)
				require.NoFileExists(t, filepath.Join(dir, "payload"))
				require.NoFileExists(t, auditPath)
				if tt.name == "scan refused" {
					require.NoFileExists(t, filepath.Join(dir, "calls"))
				}
				return
			}
			require.Equal(t, "https://github.com/owner/repo/pull/12\n", out)
			require.Empty(t, errOut)
			calls, err := os.ReadFile(filepath.Join(dir, "calls"))
			require.NoError(t, err)
			require.Equal(t, "api user --jq .login\n"+
				"api --method PUT repos/owner/repo/pulls/12/reviews/42 --input - --jq .html_url\n", string(calls))
			payload, err := os.ReadFile(filepath.Join(dir, "payload"))
			require.NoError(t, err)
			var decoded map[string]string
			require.NoError(t, json.Unmarshal(payload, &decoded))
			require.Equal(t, map[string]string{"body": tt.body}, decoded)
			data, err := os.ReadFile(auditPath)
			require.NoError(t, err)
			var record auditRecord
			require.NoError(t, json.Unmarshal(data, &record))
			require.Equal(t, "review edit", record.Operation)
			require.Equal(t, strings.TrimSpace(out), record.URL)
			hash := sha256.Sum256([]byte(tt.body))
			require.Equal(t, hex.EncodeToString(hash[:]), record.SHA256)
		})
	}
}

func TestPREditBody(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		explicit   bool
		wantBody   bool
	}{
		{"metadata only", "", false, false},
		{"stdin body", "replacement\n", false, true},
		{"explicit empty body", "", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := setup(t)
			args := []string{"pr", "edit", "12", "-R", "owner/repo", "--base", "main"}
			if tt.explicit {
				args = append(args, "--body-file", "-")
			}
			code, _, errOut := run(t, tt.body, args...)
			require.Equal(t, 0, code, errOut)
			calls, err := os.ReadFile(filepath.Join(dir, "calls"))
			require.NoError(t, err)
			require.Contains(t, string(calls), "pr edit 12 --repo owner/repo --base main")
			require.Equal(t, tt.wantBody, strings.Contains(string(calls), "--body-file -"))
			payload, err := os.ReadFile(filepath.Join(dir, "payload"))
			require.NoError(t, err)
			require.Equal(t, tt.body, string(payload))
		})
	}
}

func TestRefusalDoesNotSend(t *testing.T) {
	dir := setup(t)
	cases := []struct {
		name, body       string
		args             []string
		envKey, envValue string
		code             int
	}{
		{"scan", "file:" + "///tmp/report", nil, "", "", 1},
		{
			"title",
			"safe",
			[]string{"pr", "create", "--title", "/home/" + "example/report", "-R", "owner/repo"},
			"",
			"",
			1,
		},
		{"private", "hidden-literal", nil, "DEVTOOLS_PRIVATE_STRINGS", "hidden-literal", 1},
		{"list missing", "safe", nil, "DEVTOOLS_PRIVATE_STRINGS_REF", "missing", 2},
		{"identity", "safe", nil, "LOGIN", "person", 1},
		{"empty allowlist", "safe", nil, "DEVTOOLS_POST_IDENTITIES", "", 1},
		{"identity failed", "safe", nil, "IDENTITY_STATUS", "1", 2},
		{"mode", "safe", nil, "DEVTOOLS_SCAN_MODE", "typo", 2},
		{"usage", "safe", []string{"pr", "comment", "12", "--body", "text"}, "", "", 2},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envKey != "" {
				t.Setenv(tt.envKey, tt.envValue)
			}
			args := tt.args
			if args == nil {
				args = []string{"pr", "comment", "12", "-R", "owner/repo"}
			}
			code, out, errOut := run(t, tt.body, args...)
			require.Equal(t, tt.code, code)
			require.Empty(t, out)
			require.NotEmpty(t, errOut)
			require.NoFileExists(t, filepath.Join(dir, "payload"))
			require.NoFileExists(t, filepath.Join(dir, "state", "devtools", "gh-post.log"))
			require.NotContains(t, errOut, tt.body)
		})
	}
}

func TestPrivateConfiguration(t *testing.T) {
	dir := setup(t)
	machine := filepath.Join(dir, "config", "devtools")
	write(t, filepath.Join(machine, "devtools.conf"), "post_identities = agent\nscan_mode = all\n")
	write(
		t,
		filepath.Join(machine, "repo.conf"),
		"private_strings_ref = example-reference\nprivate_strings_token_command = "+filepath.Join(dir, "token")+"\n",
	)
	write(t, filepath.Join(dir, "token"), "#!/bin/sh\nprintf service-token")
	write(t, filepath.Join(dir, "op"), `#!/bin/sh
[ "$OP_SERVICE_ACCOUNT_TOKEN" = service-token ] || exit 1
printf '%s' "${PRIVATE_RESULT-hidden-literal,second-literal}"
`)
	config, err := settings("owner/repo")
	require.NoError(t, err)
	list, err := privateStrings(t.Context(), config)
	require.NoError(t, err)
	require.Equal(t, []string{"hidden-literal", "second-literal"}, list)
	t.Setenv("DEVTOOLS_PRIVATE_STRINGS", "exported, value ")
	list, err = privateStrings(t.Context(), config)
	require.NoError(t, err)
	require.Equal(t, []string{"exported", "value"}, list)
	t.Setenv("DEVTOOLS_PRIVATE_STRINGS", "")
	_, err = privateStrings(t.Context(), config)
	require.ErrorIs(t, err, errConfig)
	require.NoError(t, os.Unsetenv("DEVTOOLS_PRIVATE_STRINGS"))
	t.Setenv("PRIVATE_RESULT", "")
	_, err = privateStrings(t.Context(), config)
	require.ErrorIs(t, err, errConfig)
	config["private_strings"] = "none"
	list, err = privateStrings(t.Context(), config)
	require.NoError(t, err)
	require.Empty(t, list)
	config["private_strings"] = "required"
	delete(config, "private_strings_ref")
	_, err = privateStrings(t.Context(), config)
	require.ErrorIs(t, err, errConfig)
	delete(config, "private_strings")
	list, err = privateStrings(t.Context(), config)
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestSecretsOnlyAndApp(t *testing.T) {
	dir := setup(t)
	t.Setenv("DEVTOOLS_SCAN_MODE", "secrets-only")
	t.Setenv("DEVTOOLS_PRIVATE_STRINGS_REF", "unresolvable")
	t.Setenv("DEVTOOLS_PRIVATE_STRINGS", "hidden-literal")
	t.Setenv("DEVTOOLS_POST_IDENTITIES", "app[bot]")
	t.Setenv("GH_TOKEN", "test-placeholder")
	t.Setenv("IDENTITY_STATUS", "1")
	t.Setenv("GRAPHQL_STATUS", "0")
	t.Setenv("SILENT", "1")
	code, out, errOut := run(
		t,
		"Reviewed head: "+reviewHead+"\n/home/"+"example/report hidden-literal",
		"pr",
		"review",
		"12",
		"--approve",
		"--commit", reviewHead,
		"-R",
		"owner/repo",
	)
	require.Equal(t, 0, code, errOut)
	require.Contains(t, out, "/pull/12")
	require.NoError(t, os.Remove(filepath.Join(dir, "payload")))
	code, _, errOut = run(
		t,
		"Reviewed head: "+reviewHead+"\nop:"+"//vault/item",
		"pr",
		"review",
		"12",
		"--approve",
		"--commit",
		reviewHead,
		"-R",
		"owner/repo",
	)
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "secret line 2")
	require.NoFileExists(t, filepath.Join(dir, "payload"))
}

func TestEnvironmentFailures(t *testing.T) {
	dir := setup(t)
	args := []string{"pr", "comment", "12", "-R", "owner/repo"}
	code, _, _ := run(t, "safe", append(args, "--body-file", filepath.Join(dir, "absent"))...)
	require.Equal(t, 2, code)
	code, _, _ = run(t, "safe", "pr", "comment", "12", "-R", "bad/repo/target")
	require.Equal(t, 2, code)
	t.Setenv("WRITE_STATUS", "1")
	code, out, errOut := run(t, "should never echo", args...)
	require.Equal(t, 2, code)
	require.Empty(t, out)
	require.NotContains(t, errOut, "should never echo")
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "gh"))
	code, _, errOut = run(t, "safe", args...)
	require.Equal(t, 2, code)
	require.Contains(t, errOut, "cannot open audit")
}

func TestOriginAndSettings(t *testing.T) {
	dir := setup(t)
	write(t, filepath.Join(dir, "git"), "#!/bin/sh\necho git@github.com:owner/repo.git\n")
	repo, err := resolveRepo(t.Context(), "")
	require.NoError(t, err)
	require.Equal(t, "owner/repo", repo)
	t.Setenv("DEVTOOLS_POST_IDENTITIES", "")
	config, err := settings(repo)
	require.NoError(t, err)
	require.Empty(t, config["post_identities"])
	write(t, filepath.Join(dir, "config", "devtools", "devtools.conf"), "scan_mode = all\nscan_mode = ignored\n")
	write(t, filepath.Join(dir, "config", "devtools", "repo.conf"), "# comment\nscan_mode = secrets-only\n")
	config, err = settings(repo)
	require.NoError(t, err)
	require.Equal(t, "secrets-only", config["scan_mode"])
	t.Setenv("DEVTOOLS_SCAN_MODE", "all")
	config, err = settings(repo)
	require.NoError(t, err)
	require.Equal(t, "all", config["scan_mode"])
}

type brokenIO struct{}

func (brokenIO) Read([]byte) (int, error)  { return 0, io.ErrUnexpectedEOF }
func (brokenIO) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestInputAndAuditErrors(t *testing.T) {
	t.Parallel()
	_, err := readBody("", brokenIO{})
	require.ErrorIs(t, err, errEnvironment)
	require.ErrorIs(t, appendAudit(brokenIO{}, "pr comment", "url", []byte("safe")), io.ErrClosedPipe)
	require.NotEmpty(
		t,
		cleanEnv(
			[]string{
				"GH_HOST=other",
				"GH_REPO=elsewhere",
				"GH_DEBUG=api",
				"GH_PAGER=bad",
				"GH_PROMPT_DISABLED=0",
				"PATH=bin",
			},
		),
	)
	var out bytes.Buffer
	require.Equal(t, 2, Run(t.Context(), []string{}, strings.NewReader(""), &out, &out))
}

func TestAuditLocations(t *testing.T) {
	dir := setup(t)
	t.Setenv("XDG_STATE_HOME", "")
	log, err := openAudit()
	require.NoError(t, err)
	require.NoError(t, log.Close())
	require.FileExists(t, filepath.Join(dir, ".local", "state", "devtools", "gh-post.log"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "alternate"))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "alternate", "devtools", "gh-post.log"), 0o700))
	_, err = openAudit()
	require.Error(t, err)
}

const reviewHead = "0123456789abcdef0123456789abcdef01234567"

func TestHelp(t *testing.T) {
	setup(t)
	code, out, errOut := run(t, "", "--help")
	require.Equal(t, 0, code, errOut)
	require.Empty(t, errOut)
	for _, operation := range []string{
		"issue create", "issue comment", "issue edit", "pr create", "pr edit",
		"pr comment", "pr review", "pr ready", "pr close", "comment edit", "review edit", "release create", "release edit",
	} {
		require.Contains(t, out, operation)
		values, switches, ok := allowed(operation)
		require.True(t, ok)
		for flag := range values {
			require.Contains(t, out, flag)
		}
		for flag := range switches {
			require.Contains(t, out, flag)
		}
	}
	for _, flag := range []string{"--help", "--check-deps", "--commit"} {
		require.Contains(t, out, flag)
	}
}

func TestCheckDeps(t *testing.T) {
	for _, present := range [][]string{{"git", "gh"}, {"git"}, {"gh"}, {}} {
		t.Run(strings.Join(present, "+"), func(t *testing.T) {
			dir := setup(t)
			bin := filepath.Join(dir, "bin")
			require.NoError(t, os.Mkdir(bin, 0o700))
			for _, name := range present {
				write(t, filepath.Join(bin, name), "#!/bin/sh\nexit 99\n")
			}
			t.Setenv("PATH", bin)
			code, _, errOut := run(t, "", "--check-deps")
			want := 0
			for _, name := range []string{"git", "gh"} {
				if _, err := os.Stat(filepath.Join(bin, name)); os.IsNotExist(err) {
					want = 2
					require.Contains(t, errOut, name)
				}
			}
			require.Equal(t, want, code, errOut)
		})
	}
}

func TestInformationalFlagsStayOffline(t *testing.T) {
	for _, flag := range []string{"--help", "--check-deps"} {
		t.Run(flag, func(t *testing.T) {
			dir := setup(t)
			write(t, filepath.Join(dir, "git"), "#!/bin/sh\necho called >> \"$CALLS\"\nexit 99\n")
			t.Setenv("DEVTOOLS_SCAN_MODE", "invalid")
			var out, errOut bytes.Buffer
			code := Run(t.Context(), []string{flag}, brokenIO{}, &out, &errOut)
			require.Equal(t, 0, code, errOut.String())
			require.NoFileExists(t, filepath.Join(dir, "calls"))
			require.NoFileExists(t, filepath.Join(dir, "payload"))
			require.NoFileExists(t, filepath.Join(dir, "state", "devtools", "gh-post.log"))
		})
	}
}

func TestReviewPinnedHead(t *testing.T) {
	for _, event := range []struct{ flag, value string }{
		{"--approve", "APPROVE"}, {"--comment", "COMMENT"}, {"--request-changes", "REQUEST_CHANGES"},
	} {
		t.Run(event.flag, func(t *testing.T) {
			dir := setup(t)
			body := "Reviewed head: " + reviewHead + "\n\nPublic review \"quoted\"\n"
			code, _, errOut := run(
				t,
				body,
				"pr",
				"review",
				"12",
				"-R",
				"owner/repo",
				event.flag,
				"--commit",
				reviewHead,
			)
			require.Equal(t, 0, code, errOut)
			calls, err := os.ReadFile(filepath.Join(dir, "calls"))
			require.NoError(t, err)
			require.Contains(t, string(calls), "pr view 12 --repo owner/repo --json headRefOid --jq .headRefOid\n")
			require.True(
				t,
				strings.HasSuffix(
					string(calls),
					"api --method POST repos/owner/repo/pulls/12/reviews --input - --jq .html_url\n",
				),
			)
			payload, err := os.ReadFile(filepath.Join(dir, "payload"))
			require.NoError(t, err)
			var decoded map[string]string
			require.NoError(t, json.Unmarshal(payload, &decoded))
			require.Equal(t, map[string]string{"commit_id": reviewHead, "event": event.value, "body": body}, decoded)
		})
	}
}

func TestReviewHeadRefusal(t *testing.T) {
	for _, tt := range []struct {
		name, head, status string
		code               int
	}{
		{"changed", strings.Repeat("a", 40), "0", 1},
		{"read failed", reviewHead, "1", 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := setup(t)
			t.Setenv("HEAD_OID", tt.head)
			t.Setenv("HEAD_STATUS", tt.status)
			code, out, errOut := run(
				t,
				"Reviewed head: "+reviewHead+"\n",
				"pr",
				"review",
				"12",
				"-R",
				"owner/repo",
				"--approve",
				"--commit",
				reviewHead,
			)
			require.Equal(t, tt.code, code, errOut)
			require.Empty(t, out)
			require.NotEmpty(t, errOut)
			require.NoFileExists(t, filepath.Join(dir, "payload"))
		})
	}
}

func TestReviewFirstLine(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		code       int
	}{
		{"plain", "Reviewed head: " + reviewHead + "\nReview", 0},
		{"backticks", "Reviewed head: `" + reviewHead + "`\r\nReview", 0},
		{"missing", "Public review", 1},
		{"wrong prefix case", "reviewed head: " + reviewHead, 1},
		{"second line", "Review\nReviewed head: " + reviewHead, 1},
		{"mismatch", "Reviewed head: " + strings.Repeat("a", 40), 1},
		{"unclosed backtick", "Reviewed head: `" + reviewHead, 1},
		{"suffix", "Reviewed head: " + reviewHead + " extra", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := setup(t)
			code, _, errOut := run(
				t,
				tt.body,
				"pr",
				"review",
				"12",
				"-R",
				"owner/repo",
				"--approve",
				"--commit",
				reviewHead,
			)
			require.Equal(t, tt.code, code, errOut)
			if tt.code != 0 {
				require.NoFileExists(t, filepath.Join(dir, "calls"))
				require.NoFileExists(t, filepath.Join(dir, "payload"))
			}
		})
	}
}

const stateViewCall = "pr view 12 --repo owner/repo --json id,state,isDraft,headRefOid,url\n"

const readyCall = "api graphql -f query=mutation($id:ID!){markPullRequestReadyForReview(input:{pullRequestId:$id}){pullRequest{url}}} -f id=PR_example\n"

const closeCall = "pr close 12 --repo owner/repo\n"

const reasonCommentCall = "pr comment 12 --repo owner/repo --body-file -\n"

const commentsCall = "api --method GET repos/owner/repo/issues/12/comments --paginate --slurp\n"

func setupPRState(t *testing.T, verb string) string {
	t.Helper()
	dir := setup(t)
	for _, key := range []string{"STATE_STATUS", "READ_BACK_STATUS", "MUTATE_STATUS", "COMMENT_STATUS", "COMMENTS_STATUS"} {
		t.Setenv(key, "0")
	}
	t.Setenv("FIXTURE", dir)
	writePRState(t, dir, "initial", "OPEN", true, reviewHead)
	state, draft := "OPEN", false
	if verb == "close" {
		state, draft = "CLOSED", true
	}
	writePRState(t, dir, "after", state, draft, reviewHead)
	write(t, filepath.Join(dir, "reason"), "Superseded by another change.\n")
	write(t, filepath.Join(dir, "comments"), "[[]]")
	write(
		t,
		filepath.Join(dir, "new-comment"),
		`[[{"body":"Superseded by another change.\n","user":{"login":"agent"}}]]`,
	)
	write(t, filepath.Join(dir, "gh"), `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
case "$*" in
	'api user --jq .login') echo "${LOGIN:-agent}"; exit "${IDENTITY_STATUS:-0}" ;;
	'api graphql -f query={viewer{login}} --jq .data.viewer.login') echo 'app[bot]'; exit "${GRAPHQL_STATUS:-1}" ;;
	'pr view 12 --repo owner/repo --json id,state,isDraft,headRefOid,url')
		if [ -f "$FIXTURE/viewed" ]; then
			cat "$FIXTURE/after"; exit "$READ_BACK_STATUS"
		fi
		touch "$FIXTURE/viewed"
		cat "$FIXTURE/initial"; exit "$STATE_STATUS" ;;
	'api --method GET repos/owner/repo/issues/12/comments --paginate --slurp')
		cat "$FIXTURE/comments"; exit "$COMMENTS_STATUS" ;;
	'pr comment 12 --repo owner/repo --body-file -')
		printf '%s\n' "$*" >> "$FIXTURE/writes"
		cat > "$PAYLOAD"
		[ "$COMMENT_STATUS" = 0 ] || exit "$COMMENT_STATUS"
		cp "$FIXTURE/new-comment" "$FIXTURE/comments"
		echo 'https://github.com/owner/repo/pull/12#issuecomment-1'; exit 0 ;;
	'api graphql -f query=mutation('*|'pr close 12 --repo owner/repo')
		printf '%s\n' "$*" >> "$FIXTURE/writes"
		exit "$MUTATE_STATUS" ;;
	*) exit 99 ;;
esac
`)
	return dir
}

func writePRState(t *testing.T, dir, file, state string, draft bool, head string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"id": "PR_example", "state": state, "isDraft": draft, "headRefOid": head,
		"url":    "https://github.com/owner/repo/pull/12",
		"author": map[string]string{"login": "another-author"},
	})
	require.NoError(t, err)
	write(t, filepath.Join(dir, file), string(data))
}

func stateArgs(dir, verb string) []string {
	args := []string{"pr", verb, "12", "--repo", "owner/repo", "--head", reviewHead}
	if verb == "close" {
		args = append(args, "--comment-file", filepath.Join(dir, "reason"))
	}
	return args
}

func fileText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(data)
}

func TestPRStateAC1AC2AC10(t *testing.T) {
	for _, verb := range []string{"ready", "close"} {
		t.Run(verb, func(t *testing.T) {
			dir := setupPRState(t, verb)
			var out, errOut bytes.Buffer
			// Neither verb may read body input, even when stdin fails.
			code := Run(t.Context(), stateArgs(dir, verb), brokenIO{}, &out, &errOut)
			require.Equal(t, 0, code, errOut.String())
			require.Equal(t, "https://github.com/owner/repo/pull/12\n", out.String())
			require.Empty(t, errOut.String())
			writes := readyCall
			calls := "api user --jq .login\n" + stateViewCall
			if verb == "close" {
				writes = reasonCommentCall + closeCall
				calls += commentsCall
				require.Equal(t, "Superseded by another change.\n", fileText(t, filepath.Join(dir, "payload")))
			}
			require.Equal(t, writes, fileText(t, filepath.Join(dir, "writes")))
			require.Equal(t, calls+writes+stateViewCall, fileText(t, filepath.Join(dir, "calls")))
			require.Contains(t, fileText(t, filepath.Join(dir, "state", "devtools", "gh-post.log")), "pr "+verb)
		})
	}
}

func TestPRStateAC3(t *testing.T) {
	for _, tt := range []struct {
		name, state, head, message string
		draft                      bool
	}{
		{"head", "OPEN", strings.Repeat("a", 40), "PR head does not match --head", true},
		{"closed", "CLOSED", reviewHead, "PR is not open", true},
		{"merged", "MERGED", reviewHead, "PR is not open", true},
		{"non-draft", "OPEN", reviewHead, "PR is not a draft", false},
	} {
		for _, verb := range []string{"ready", "close"} {
			if tt.name == "non-draft" && verb == "close" {
				continue
			}
			t.Run(verb+"/"+tt.name, func(t *testing.T) {
				dir := setupPRState(t, verb)
				writePRState(t, dir, "initial", tt.state, tt.draft, tt.head)
				code, out, errOut := run(t, "", stateArgs(dir, verb)...)
				require.Equal(t, 1, code)
				require.Empty(t, out)
				require.Equal(t, "gh-post: "+tt.message+"\n", errOut)
				require.NoFileExists(t, filepath.Join(dir, "writes"))
				require.Equal(t, "api user --jq .login\n"+stateViewCall, fileText(t, filepath.Join(dir, "calls")))
			})
		}
	}
}

func TestPRStateAC4(t *testing.T) {
	for _, verb := range []string{"ready", "close"} {
		for _, head := range []string{"", "abc", strings.Repeat("a", 39), strings.Repeat("a", 41), strings.Repeat("g", 40)} {
			t.Run(verb+"/"+head, func(t *testing.T) {
				dir := setupPRState(t, verb)
				args := []string{"pr", verb, "12", "--repo", "owner/repo"}
				if head != "" {
					args = append(args, "--head", head)
				}
				if verb == "close" {
					args = append(args, "--comment-file", filepath.Join(dir, "reason"))
				}
				code, _, errOut := run(t, "", args...)
				require.Equal(t, 2, code)
				require.Equal(t, "gh-post: "+errUsage.Error()+"\n", errOut)
				require.NoFileExists(t, filepath.Join(dir, "writes"))
				require.NoFileExists(t, filepath.Join(dir, "calls"))
			})
		}
	}
}

func TestPRStateAC4CaseInsensitive(t *testing.T) {
	for _, verb := range []string{"ready", "close"} {
		t.Run(verb, func(t *testing.T) {
			dir := setupPRState(t, verb)
			args := stateArgs(dir, verb)
			args[6] = strings.ToUpper(reviewHead)
			writePRState(t, dir, "initial", "OPEN", true, strings.ToUpper(reviewHead))
			code, _, errOut := run(t, "", args...)
			require.Equal(t, 0, code, errOut)
		})
	}
}

func TestPRStateAC5(t *testing.T) {
	for _, tt := range []struct {
		name, reason, message string
		code                  int
	}{
		{"missing", "", "", 2},
		{"empty", "", "gh-post: close reason must not be empty\n", 2},
		{"whitespace", " \n", "gh-post: close reason must not be empty\n", 2},
		{"scan", "hidden-literal", "gh-post: private-string line 1\n", 1},
		{"unreadable", "", "gh-post: cannot read body\n", 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupPRState(t, "close")
			t.Setenv("DEVTOOLS_PRIVATE_STRINGS", "hidden-literal")
			args := stateArgs(dir, "close")
			write(t, filepath.Join(dir, "reason"), tt.reason)
			if tt.name == "missing" {
				args = args[:len(args)-2]
			}
			if tt.name == "unreadable" {
				require.NoError(t, os.Remove(filepath.Join(dir, "reason")))
			}
			code, out, errOut := run(t, "unused", args...)
			require.Equal(t, tt.code, code)
			require.Empty(t, out)
			if tt.message != "" {
				require.Equal(t, tt.message, errOut)
			}
			require.NoFileExists(t, filepath.Join(dir, "writes"))
			require.NoFileExists(t, filepath.Join(dir, "calls"))
		})
	}
}

func TestPRStateAC6(t *testing.T) {
	for _, verb := range []string{"ready", "close"} {
		t.Run(verb, func(t *testing.T) {
			dir := setupPRState(t, verb)
			code, out, errOut := run(t, "ignored", append(stateArgs(dir, verb), "--dry-run")...)
			require.Equal(t, 0, code, errOut)
			require.Contains(t, out, "would "+verb+" https://github.com/owner/repo/pull/12")
			if verb == "close" {
				require.Contains(t, out, "reason comment")
			}
			require.Empty(t, errOut)
			require.NoFileExists(t, filepath.Join(dir, "writes"))
			require.Equal(t, "api user --jq .login\n"+stateViewCall, fileText(t, filepath.Join(dir, "calls")))
		})
	}
}

func TestPRStateAC7(t *testing.T) {
	dir := setupPRState(t, "close")
	t.Setenv("MUTATE_STATUS", "1")
	writePRState(t, dir, "after", "OPEN", true, reviewHead)
	code, _, errOut := run(t, "", stateArgs(dir, "close")...)
	require.Equal(t, 2, code)
	require.Equal(
		t,
		"gh-post: reason comment was posted; close failed and PR is still open; check GitHub before retrying\n",
		errOut,
	)
	require.Equal(t, reasonCommentCall+closeCall, fileText(t, filepath.Join(dir, "writes")))
	require.Equal(
		t,
		"api user --jq .login\n"+stateViewCall+commentsCall+reasonCommentCall+closeCall+stateViewCall,
		fileText(t, filepath.Join(dir, "calls")),
	)
	require.NoError(t, os.Remove(filepath.Join(dir, "viewed")))
	t.Setenv("MUTATE_STATUS", "0")
	writePRState(t, dir, "after", "CLOSED", true, reviewHead)
	code, _, errOut = run(t, "", stateArgs(dir, "close")...)
	require.Equal(t, 0, code, errOut)
	require.Equal(t, reasonCommentCall+closeCall+closeCall, fileText(t, filepath.Join(dir, "writes")))
}

func TestPRStateAC8(t *testing.T) {
	for _, verb := range []string{"ready", "close"} {
		for _, failure := range []string{"head", "read", "state", "draft"} {
			t.Run(verb+"/"+failure, func(t *testing.T) {
				dir := setupPRState(t, verb)
				state, draft, head := "OPEN", false, reviewHead
				message, want := "PR read-back contradicts requested state", 1
				switch failure {
				case "head":
					head, message = strings.Repeat("a", 40), "PR read-back head does not match --head"
				case "read":
					t.Setenv("READ_BACK_STATUS", "1")
					message, want = "cannot read PR back", 2
				case "state":
					state = "MERGED"
				case "draft":
					draft = true
				}
				writePRState(t, dir, "after", state, draft, head)
				code, out, errOut := run(t, "", stateArgs(dir, verb)...)
				require.Equal(t, want, code)
				require.Empty(t, out)
				require.Equal(t, "gh-post: "+message+"; do not retry or undo automatically\n", errOut)
				writes := readyCall
				if verb == "close" {
					writes = reasonCommentCall + closeCall
				}
				require.Equal(t, writes, fileText(t, filepath.Join(dir, "writes")))
			})
		}
	}
}

func TestPRStateAC9(t *testing.T) {
	for _, verb := range []string{"ready", "close"} {
		t.Run(verb, func(t *testing.T) {
			dir := setupPRState(t, verb)
			t.Setenv("LOGIN", "unapproved")
			code, out, errOut := run(t, "", stateArgs(dir, verb)...)
			require.Equal(t, 1, code)
			require.Empty(t, out)
			require.Equal(t, "gh-post: identity refused\n", errOut)
			require.NoFileExists(t, filepath.Join(dir, "writes"))
			require.Equal(t, "api user --jq .login\n", fileText(t, filepath.Join(dir, "calls")))
		})
	}
}

func TestPRStateAC10ReasonLimit(t *testing.T) {
	for _, size := range []int{65536, 65537} {
		t.Run(strings.Repeat("x", size-65535), func(t *testing.T) {
			dir := setupPRState(t, "close")
			write(t, filepath.Join(dir, "reason"), strings.Repeat("x", size))
			code, _, errOut := run(t, "", stateArgs(dir, "close")...)
			if size == 65536 {
				require.Equal(t, 0, code, errOut)
				require.Equal(t, reasonCommentCall+closeCall, fileText(t, filepath.Join(dir, "writes")))
			} else {
				require.Equal(t, 2, code)
				require.Equal(t, "gh-post: close reason exceeds comment size limit\n", errOut)
				require.NoFileExists(t, filepath.Join(dir, "writes"))
				require.NoFileExists(t, filepath.Join(dir, "calls"))
			}
		})
	}
}

func TestPRStateBeforeWriteFailures(t *testing.T) {
	for _, tt := range []struct{ name, message string }{
		{"state", "cannot read PR state"},
		{"invalid state", "cannot read PR state"},
		{"comments", "cannot read PR comments"},
		{"invalid comments", "cannot read PR comments"},
		{"identity", "cannot read gh identity"},
		{"comment", "gh write failed; check GitHub before retrying"},
		{"audit", "cannot open audit log"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupPRState(t, "close")
			switch tt.name {
			case "state":
				t.Setenv("STATE_STATUS", "1")
			case "invalid state":
				write(t, filepath.Join(dir, "initial"), "{}")
			case "comments":
				t.Setenv("COMMENTS_STATUS", "1")
			case "invalid comments":
				write(t, filepath.Join(dir, "comments"), "invalid")
			case "identity":
				t.Setenv("IDENTITY_STATUS", "1")
				t.Setenv("GRAPHQL_STATUS", "1")
			case "comment":
				t.Setenv("COMMENT_STATUS", "1")
			case "audit":
				t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "gh"))
			}
			code, out, errOut := run(t, "", stateArgs(dir, "close")...)
			require.Equal(t, 2, code)
			require.Empty(t, out)
			require.Equal(t, "gh-post: "+tt.message+"\n", errOut)
			if tt.name == "comment" {
				require.Equal(t, reasonCommentCall, fileText(t, filepath.Join(dir, "writes")))
			} else {
				require.NoFileExists(t, filepath.Join(dir, "writes"))
			}
		})
	}
}

func TestPRStateCommentIdentity(t *testing.T) {
	for _, tt := range []struct{ name, comments, writes string }{
		{"same identity later page", `[[{"body":"different","user":{"login":"agent"}}],[{"body":"Superseded by another change.\n","user":{"login":"agent"}}]]`, closeCall},
		{"different identity", `[[{"body":"Superseded by another change.\n","user":{"login":"someone-else"}}]]`, reasonCommentCall + closeCall},
		{"different text", `[[{"body":"different","user":{"login":"agent"}}]]`, reasonCommentCall + closeCall},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupPRState(t, "close")
			write(t, filepath.Join(dir, "comments"), tt.comments)
			code, _, errOut := run(t, "", stateArgs(dir, "close")...)
			require.Equal(t, 0, code, errOut)
			require.Equal(t, tt.writes, fileText(t, filepath.Join(dir, "writes")))
		})
	}
}

func TestPRStateFlagBoundaries(t *testing.T) {
	for _, verb := range []string{"ready", "close"} {
		for _, extra := range [][]string{
			{"--body-file", "-"},
			{"--body", "text"},
			{"--head", reviewHead},
			{"--delete-branch"},
			{"--undo"},
			{"--reopen"},
			{"13"},
		} {
			t.Run(verb+"/"+strings.Join(extra, " "), func(t *testing.T) {
				dir := setupPRState(t, verb)
				code, _, _ := run(t, "", append(stateArgs(dir, verb), extra...)...)
				require.Equal(t, 2, code)
				require.NoFileExists(t, filepath.Join(dir, "writes"))
				require.NoFileExists(t, filepath.Join(dir, "calls"))
			})
		}
	}
	for _, args := range [][]string{
		{"pr", "ready", "12", "--head", reviewHead, "--comment-file", "reason"},
		{"pr", "close", "12", "--head", reviewHead, "--comment-file", "-"},
		{"pr", "comment", "12", "--dry-run"},
		{"pr", "edit", "12", "--dry-run"},
		{"pr", "review", "12", "--approve", "--commit", reviewHead, "--dry-run"},
		{"issue", "comment", "12", "--dry-run"},
		{"release", "edit", "v1", "--dry-run"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := setupPRState(t, "ready")
			code, _, _ := run(t, "", args...)
			require.Equal(t, 2, code)
			require.NoFileExists(t, filepath.Join(dir, "writes"))
			require.NoFileExists(t, filepath.Join(dir, "calls"))
		})
	}
}

func TestPRStateAC10MutationFailure(t *testing.T) {
	for _, verb := range []string{"ready", "close"} {
		t.Run(verb, func(t *testing.T) {
			dir := setupPRState(t, verb)
			t.Setenv("MUTATE_STATUS", "1")
			code, out, errOut := run(t, "", stateArgs(dir, verb)...)
			require.Equal(t, 2, code)
			require.Empty(t, out)
			require.Equal(t, "gh-post: PR state write failed; check GitHub before retrying\n", errOut)
			writes := readyCall
			if verb == "close" {
				writes = reasonCommentCall + closeCall
			}
			require.Equal(t, writes, fileText(t, filepath.Join(dir, "writes")))
			require.True(t, strings.HasSuffix(fileText(t, filepath.Join(dir, "calls")), stateViewCall))
		})
	}
}
