// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIssueBodyEditAC1AC5(t *testing.T) {
	const body = "Public replacement.\n\nKeep exact whitespace.\n"
	for _, form := range []string{"file", "explicit stdin", "implicit stdin", "closed", "silent", "type"} {
		t.Run(form, func(t *testing.T) {
			t.Setenv(fakeGHEnv, "1")
			dir := setupIssueType(t, "Feature")
			args := []string{"issue", "edit", "12", "--repo", "owner/repo"}
			input := body
			switch form {
			case "file", "type":
				path := filepath.Join(dir, "body")
				write(t, path, body)
				args = append(args, "--body-file", path)
				input = "unused stdin"
				if form == "type" {
					args = append(args, "--type", "Feature")
				}
			case "explicit stdin":
				args = append(args, "--body-file", "-")
			case "closed":
				write(
					t,
					filepath.Join(dir, "issue-state"),
					`{"state":"CLOSED","url":"https://github.com/owner/repo/issues/12"}`,
				)
				args = append(args, "--allow-closed")
			case "silent":
				t.Setenv("SILENT", "1")
			}
			code, out, errOut := run(t, input, args...)
			require.Equal(t, 0, code, errOut)
			require.Contains(t, out, "https://github.com/owner/repo/issues/12\n")
			require.Empty(t, errOut)
			require.Equal(t, body, fileText(t, filepath.Join(dir, "payload")))
			calls := fileText(t, filepath.Join(dir, "calls"))
			require.Contains(t, calls, "issue edit 12 --repo owner/repo --body-file -\n")
			require.NotContains(t, calls, "--allow-closed")
			require.Less(t, strings.Index(calls, "api user"), strings.Index(calls, "issue view"))
			require.Less(t, strings.Index(calls, "issue view"), strings.Index(calls, "issue edit"))
			lines := strings.Split(
				strings.TrimSpace(fileText(t, filepath.Join(dir, "state", "devtools", "gh-post.log"))),
				"\n",
			)
			var record auditRecord
			require.NoError(t, json.Unmarshal([]byte(lines[0]), &record))
			require.Equal(t, "issue edit", record.Operation)
			require.Equal(t, "https://github.com/owner/repo/issues/12", record.URL)
			hash := sha256.Sum256([]byte(body))
			require.Equal(t, hex.EncodeToString(hash[:]), record.SHA256)
			_, err := time.Parse(time.RFC3339Nano, record.Time)
			require.NoError(t, err)
			require.True(t, strings.HasSuffix(record.Time, "Z"), "audit timestamp must use UTC")
			wantRecords := 1
			if form == "type" {
				wantRecords++
			}
			require.Len(t, lines, wantRecords)
			require.NotContains(t, strings.Join(lines, "\n"), "Public replacement")
		})
	}
}

func TestIssueBodyEditAC2(t *testing.T) {
	for _, body := range []string{"", " \n\t"} {
		for _, form := range []string{"file", "explicit stdin", "implicit stdin"} {
			t.Run(fmt.Sprintf("%q/%s", body, form), func(t *testing.T) {
				t.Setenv(fakeGHEnv, "1")
				dir := setupIssueType(t, "Feature")
				args := []string{"issue", "edit", "12", "--repo", "owner/repo"}
				switch form {
				case "file":
					write(t, filepath.Join(dir, "body"), body)
					args = append(
						args,
						"--body-file",
						filepath.Join(dir, "body"),
						"--title",
						"Metadata cannot clear a body",
					)
				case "explicit stdin":
					args = append(args, "--body-file", "-", "--title", "Metadata cannot clear a body")
				}
				code, _, errOut := run(t, body, args...)
				require.Equal(t, 2, code)
				require.Contains(t, errOut, "issue body must not be empty")
				require.NoFileExists(t, filepath.Join(dir, "calls"))
			})
		}
	}
}

func TestIssueBodyEditInputFailureAC2(t *testing.T) {
	for _, form := range []string{"missing file", "directory"} {
		t.Run(form, func(t *testing.T) {
			t.Setenv(fakeGHEnv, "1")
			dir := setupIssueType(t, "Feature")
			path := filepath.Join(dir, "missing-body")
			if form == "directory" {
				path = dir
			}
			code, out, errOut := run(t, "", "issue", "edit", "12", "--repo", "owner/repo", "--body-file", path)
			require.Equal(t, 2, code)
			require.Empty(t, out)
			require.Contains(t, errOut, "cannot read body")
			require.NoFileExists(t, filepath.Join(dir, "calls"))
		})
	}
}

func TestIssueBodyEditMetadataAC2(t *testing.T) {
	t.Setenv(fakeGHEnv, "1")
	dir := setupIssueType(t, "Feature")
	code, _, errOut := run(t, "", "issue", "edit", "12", "--repo", "owner/repo", "--title", "New title")
	require.Equal(t, 0, code, errOut)
	require.NoFileExists(t, filepath.Join(dir, "payload"))
	require.Contains(t, fileText(t, filepath.Join(dir, "calls")), "issue edit 12 --repo owner/repo --title New title\n")
}

func TestIssueBodyEditRefusalsAC3AC4AC5(t *testing.T) {
	for _, tt := range []struct {
		name, state, body, login, message string
		code                              int
	}{
		{"closed", `{"state":"CLOSED","url":"https://github.com/owner/repo/issues/12"}`, "Public body", "", "--allow-closed", 1},
		{"PR", `{"state":"OPEN","url":"https://github.com/owner/repo/pull/12"}`, "Public body", "", "cannot read issue state", 2},
		{"invalid state", `{"state":"OTHER","url":"https://github.com/owner/repo/issues/12"}`, "Public body", "", "cannot read issue state", 2},
		{"missing", "null", "Public body", "", "cannot read issue state", 2},
		{"malformed", "{", "Public body", "", "cannot read issue state", 2},
		{"wrong repo", `{"state":"OPEN","url":"https://github.com/other/repo/issues/12"}`, "Public body", "", "cannot read issue state", 2},
		{"wrong issue", `{"state":"OPEN","url":"https://github.com/owner/repo/issues/13"}`, "Public body", "", "cannot read issue state", 2},
		{"read failure", "", "Public body", "", "cannot read issue state", 2},
		{"scan", "", "hidden-literal", "", "private-string", 1},
		{"secret", "", "ghp_" + strings.Repeat("a", 30), "", "secret", 1},
		{"identity", "", "Public body", "stranger", "identity refused", 1},
		{"write", "", "Public body", "", "gh write failed", 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(fakeGHEnv, "1")
			dir := setupIssueType(t, "Feature")
			t.Setenv("DEVTOOLS_PRIVATE_STRINGS", "hidden-literal")
			t.Setenv("LOGIN", tt.login)
			write(t, filepath.Join(dir, "issue-state"), tt.state)
			if tt.name == "write" {
				t.Setenv("WRITE_STATUS", "1")
			}
			if tt.name == "read failure" {
				write(t, filepath.Join(dir, "missing"), "")
			}
			args := []string{"issue", "edit", "12", "--repo", "owner/repo"}
			if tt.name == "PR" {
				args = append(args, "--allow-closed")
			}
			code, out, errOut := run(t, tt.body, args...)
			require.Equal(t, tt.code, code, errOut)
			require.Empty(t, out)
			require.Contains(t, errOut, tt.message)
			if tt.name != "write" {
				require.NoFileExists(t, filepath.Join(dir, "payload"))
				require.NotContains(t, fileText(t, filepath.Join(dir, "calls")), "issue edit")
				require.NoFileExists(t, filepath.Join(dir, "type-writes"))
			} else {
				require.Equal(t, 1, strings.Count(fileText(t, filepath.Join(dir, "calls")), "issue edit"))
			}
			require.Empty(t, fileText(t, filepath.Join(dir, "state", "devtools", "gh-post.log")))
		})
	}
}

func TestIssueBodyEditHelpAC6(t *testing.T) {
	t.Parallel()
	for _, text := range []string{usage, fileText(t, "../../docs/gh-post.md")} {
		for _, term := range []string{"issue edit", "--body-file", "--allow-closed", "empty"} {
			require.Contains(t, text, term)
		}
	}
}
