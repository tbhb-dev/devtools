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
	for _, key := range []string{"DEVTOOLS_PRIVATE_STRINGS", "DEVTOOLS_PRIVATE_STRINGS_REF", "DEVTOOLS_PRIVATE_STRINGS_TOKEN_COMMAND", "DEVTOOLS_SCAN_MODE", "DEVTOOLS_POST_IDENTITIES", "GH_TOKEN", "OP_SERVICE_ACCOUNT_TOKEN", "LOGIN", "IDENTITY_STATUS", "GRAPHQL_STATUS", "WRITE_STATUS", "SILENT", "PRIVATE_RESULT"} {
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
		"/home/"+"example/report hidden-literal",
		"pr",
		"review",
		"12",
		"--approve",
		"-R",
		"owner/repo",
	)
	require.Equal(t, 0, code, errOut)
	require.Contains(t, out, "/pull/12")
	require.NoError(t, os.Remove(filepath.Join(dir, "payload")))
	code, _, errOut = run(t, "op:"+"//vault/item", "pr", "review", "12", "--approve", "-R", "owner/repo")
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "secret line 1")
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
