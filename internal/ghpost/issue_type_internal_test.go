// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const fakeGHEnv = "GHPOST_FAKE_GH"

// TestMain runs the test binary as the GitHub CLI when the fixture links it to
// that name and sets the fake-CLI variable. The command still runs as a child process.
func TestMain(m *testing.M) {
	if os.Getenv(fakeGHEnv) == "1" {
		os.Exit(fakeGH(os.Args[1:], os.Stdin, os.Stdout))
	}
	os.Exit(m.Run())
}

func fakeGH(args []string, stdin io.Reader, stdout io.Writer) int {
	root, err := os.OpenRoot(filepath.Dir(os.Getenv("CALLS")))
	if err != nil {
		return 1
	}
	defer root.Close()
	code, err := serveFakeGH(root, strings.Join(args, " "), stdin, stdout)
	if err != nil {
		return 1
	}
	return code
}

func serveFakeGH(root *os.Root, joined string, stdin io.Reader, stdout io.Writer) (int, error) {
	if err := fakeAppend(root, "calls", joined+"\n"); err != nil {
		return 1, err
	}
	switch {
	case joined == "api user --jq .login":
		login := os.Getenv("LOGIN")
		if login == "" {
			login = "agent"
		}
		fmt.Fprintln(stdout, login)
	case strings.Contains(joined, "issueTypes("):
		fmt.Fprint(stdout, fakeRead(root, "types"))
	case strings.Contains(joined, "issue(number:"):
		if _, err := root.Stat("missing"); err == nil {
			return 1, nil
		}
		current := fakeRead(root, "current")
		fmt.Fprintf(stdout,
			`{"id":"ISSUE","viewerCanUpdateMetadata":%s,"issueType":{"id":%q,"name":%q}}`,
			fakeRead(root, "access"), current, current)
	case strings.Contains(joined, "mutation("):
		err := fakeAppend(root, "type-writes", joined+"\n")
		if err == nil {
			err = root.WriteFile("current", []byte(fakeRead(root, "requested")), 0o600)
		}
		if err != nil {
			return 1, fmt.Errorf("record mutation: %w", err)
		}
		fmt.Fprintln(stdout, "{}")
	case joined == "issue view 12 --repo owner/repo --json state,url":
		if _, err := root.Stat("missing"); err == nil {
			return 1, nil
		}
		state := fakeRead(root, "issue-state")
		if state == "" {
			state = `{"state":"OPEN","url":"https://github.com/owner/repo/issues/12"}`
		}
		fmt.Fprint(stdout, state)
	case strings.HasPrefix(joined, "issue edit 12 --repo owner/repo"):
		return fakeIssueEditResponse(root, joined, stdin, stdout)
	default:
		return 99, nil
	}
	return 0, nil
}

func fakeIssueEditResponse(root *os.Root, joined string, stdin io.Reader, stdout io.Writer) (int, error) {
	if err := fakeIssueEdit(root, joined, stdin); err != nil {
		return 1, err
	}
	if os.Getenv("WRITE_STATUS") == "1" {
		return 1, nil
	}
	if os.Getenv("SILENT") != "1" {
		fmt.Fprintln(stdout, "https://github.com/owner/repo/issues/12")
	}
	return 0, nil
}

func fakeRead(root *os.Root, name string) string {
	data, err := root.ReadFile(name)
	if err != nil {
		return ""
	}
	return string(data)
}

func fakeAppend(root *os.Root, name, text string) error {
	file, err := root.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	_, err = file.WriteString(text)
	return errors.Join(err, file.Close())
}

func fakeIssueEdit(root *os.Root, joined string, stdin io.Reader) error {
	if !strings.HasSuffix(joined, "--body-file -") {
		return fakeAppend(root, "no-body-edits", joined+"\n")
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if err = root.WriteFile("payload", data, 0o600); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}

// setupIssueType replaces the package's shell fake with a link to the test
// binary. Each caller sets the fake-CLI variable itself.
func setupIssueType(t *testing.T, current string) string {
	t.Helper()
	dir := setup(t)
	executable, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(dir, "gh")))
	require.NoError(t, os.Symlink(executable, filepath.Join(dir, "gh")))
	write(t, filepath.Join(dir, "current"), current)
	write(t, filepath.Join(dir, "types"), `[{"id":"TYPE","name":"Feature"}]`)
	write(t, filepath.Join(dir, "access"), "true")
	write(t, filepath.Join(dir, "requested"), "Feature")
	return dir
}

func issueTypeArgs(name string) []string {
	return []string{"issue", "edit", "12", "--repo", "owner/repo", "--type", name}
}

func TestIssueEditTypeAC1(t *testing.T) {
	for _, name := range []string{"Feature", "Defect", "Chore", "Spike"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(fakeGHEnv, "1")
			dir := setupIssueType(t, "")
			write(t, filepath.Join(dir, "types"), `[{"id":"TYPE","name":"`+name+`"}]`)
			write(t, filepath.Join(dir, "requested"), name)
			code, _, errOut := run(t, "ignored", issueTypeArgs(name)...)
			require.Equal(t, 0, code, errOut)
			require.Equal(t, name, strings.TrimSpace(fileText(t, filepath.Join(dir, "current"))))
			calls := fileText(t, filepath.Join(dir, "calls"))
			require.Equal(t, 2, strings.Count(calls, "issue(number:"))
			require.Contains(t, calls, "issueTypeId=TYPE")
		})
	}
}

func TestIssueEditTypeAC2(t *testing.T) {
	t.Setenv(fakeGHEnv, "1")
	dir := setupIssueType(t, "Defect")
	code, out, errOut := run(t, "", issueTypeArgs("Feature")...)
	require.Equal(t, 0, code, errOut)
	require.Contains(t, out, "Defect")
	require.Contains(t, out, "Feature")
	require.Equal(t, "Feature", strings.TrimSpace(fileText(t, filepath.Join(dir, "current"))))
}

func TestIssueEditTypeAC3(t *testing.T) {
	t.Setenv(fakeGHEnv, "1")
	dir := setupIssueType(t, "Feature")
	code, out, errOut := run(t, "", issueTypeArgs("Feature")...)
	require.Equal(t, 0, code, errOut)
	require.Contains(t, out, "nothing changed")
	require.NoFileExists(t, filepath.Join(dir, "type-writes"))
	require.NoFileExists(t, filepath.Join(dir, "payload"))
}

func TestIssueEditTypeAC4(t *testing.T) {
	for _, name := range []string{"Unknown", "feature", "", "none"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(fakeGHEnv, "1")
			dir := setupIssueType(t, "")
			code, _, errOut := run(t, "", issueTypeArgs(name)...)
			require.Equal(t, 2, code)
			require.Contains(t, errOut, "--type")
			require.NoFileExists(t, filepath.Join(dir, "calls"))
		})
	}
}

func TestIssueEditTypeAC5(t *testing.T) {
	t.Setenv(fakeGHEnv, "1")
	dir := setupIssueType(t, "")
	write(t, filepath.Join(dir, "types"), "[]")
	code, _, errOut := run(t, "", issueTypeArgs("Feature")...)
	require.Equal(t, 2, code)
	require.Contains(t, errOut, "Feature")
	require.Contains(t, fileText(t, filepath.Join(dir, "calls")), "issueTypes(")
	require.NoFileExists(t, filepath.Join(dir, "type-writes"))
}

func TestIssueEditTypeAC6(t *testing.T) {
	t.Setenv(fakeGHEnv, "1")
	dir := setupIssueType(t, "")
	write(t, filepath.Join(dir, "missing"), "yes")
	code, _, errOut := run(t, "", issueTypeArgs("Feature")...)
	require.Equal(t, 2, code)
	require.Contains(t, errOut, "12")
	require.NoFileExists(t, filepath.Join(dir, "type-writes"))
}

func TestIssueEditTypeAC7(t *testing.T) {
	t.Setenv(fakeGHEnv, "1")
	dir := setupIssueType(t, "")
	write(t, filepath.Join(dir, "access"), "false")
	code, _, errOut := run(t, "", issueTypeArgs("Feature")...)
	require.Equal(t, 2, code)
	require.Contains(t, errOut, "owner/repo")
	require.Contains(t, fileText(t, filepath.Join(dir, "calls")), "issue(number:")
	require.NoFileExists(t, filepath.Join(dir, "type-writes"))
}

func TestIssueEditTypeAC8(t *testing.T) {
	t.Setenv(fakeGHEnv, "1")
	dir := setupIssueType(t, "")
	code, _, errOut := run(t, "ignored", issueTypeArgs("Feature")...)
	require.Equal(t, 0, code, errOut)
	require.Equal(t, 1, strings.Count(fileText(t, filepath.Join(dir, "type-writes")), "mutation("))
	require.NoFileExists(t, filepath.Join(dir, "payload"))
	require.NotContains(t, fileText(t, filepath.Join(dir, "calls")), "issue edit")
}

func TestIssueEditTypeAC9(t *testing.T) {
	for _, body := range []string{"Public explanation.", "ghp_" + strings.Repeat("a", 30)} {
		t.Run(body[:6], func(t *testing.T) {
			t.Setenv(fakeGHEnv, "1")
			dir := setupIssueType(t, "")
			write(t, filepath.Join(dir, "body"), body)
			args := append(issueTypeArgs("Feature"), "--body-file", filepath.Join(dir, "body"))
			code, _, errOut := run(t, "", args...)
			if strings.HasPrefix(body, "ghp_") {
				require.Equal(t, 1, code, errOut)
				require.Contains(t, errOut, "secret")
				require.NoFileExists(t, filepath.Join(dir, "type-writes"))
				require.NoFileExists(t, filepath.Join(dir, "payload"))
			} else {
				require.Equal(t, 0, code, errOut)
				require.FileExists(t, filepath.Join(dir, "type-writes"))
				require.Equal(t, body, fileText(t, filepath.Join(dir, "payload")))
			}
		})
	}
}

func TestIssueEditTypeAC10(t *testing.T) {
	for _, outcome := range []string{"success", "unchanged", "invalid", "undefined", "missing", "access", "secret", "identity", "input"} {
		t.Run(outcome, func(t *testing.T) {
			t.Setenv(fakeGHEnv, "1")
			dir := setupIssueType(t, "")
			name := "Feature"
			args := issueTypeArgs(name)
			switch outcome {
			case "unchanged":
				write(t, filepath.Join(dir, "current"), name)
			case "invalid":
				name = "Unknown"
				args = issueTypeArgs(name)
			case "undefined":
				write(t, filepath.Join(dir, "types"), "[]")
			case "missing":
				write(t, filepath.Join(dir, "missing"), "yes")
			case "access":
				write(t, filepath.Join(dir, "access"), "false")
			case "secret":
				write(t, filepath.Join(dir, "body"), "ghp_"+strings.Repeat("a", 30))
				args = append(args, "--body-file", filepath.Join(dir, "body"))
			case "identity":
				t.Setenv("LOGIN", "stranger")
			case "input":
				args = append(args, "--body-file", filepath.Join(dir, "absent"))
			}
			code, _, _ := run(t, "", args...)
			var record struct {
				Issue   string `json:"issue"`
				Type    string `json:"type"`
				Outcome string `json:"outcome"`
			}
			lines := strings.Split(
				strings.TrimSpace(fileText(t, filepath.Join(dir, "state", "devtools", "gh-post.log"))),
				"\n",
			)
			require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &record))
			require.Equal(t, "12", record.Issue)
			require.Equal(t, name, record.Type)
			want := "refused"
			if code == 0 {
				want = "success"
			}
			require.Equal(t, want, record.Outcome)
		})
	}
}

func TestIssueEditTypeAC11(t *testing.T) {
	t.Parallel()
	require.Contains(t, usage, "--type")
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	for _, entry := range entries {
		require.NotRegexp(t, `\.(sh|bash)$`, entry.Name())
	}
}

func TestIssueEditTypeMetadataKeepsBody(t *testing.T) {
	for _, current := range []string{"", "Feature"} {
		t.Run("current="+current, func(t *testing.T) {
			t.Setenv(fakeGHEnv, "1")
			dir := setupIssueType(t, current)
			args := append(issueTypeArgs("Feature"), "--add-label", "triaged")
			code, _, errOut := run(t, "ignored", args...)
			require.Equal(t, 0, code, errOut)
			require.NoFileExists(t, filepath.Join(dir, "payload"))
			require.Contains(t, fileText(t, filepath.Join(dir, "no-body-edits")), "--add-label triaged")
		})
	}
}
