// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//nolint:paralleltest // Fake commands and configuration tests change the process environment.
package ghpost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIncompleteBodyRefusal(t *testing.T) {
	for _, tt := range []struct{ name, operation, body, diagnostic string }{
		{"missing round", "issue", "Refinement: READY\nGaps", "missing closing line Round: <n> of 3 (n from 1 to 3)"},
		{"round zero", "issue", "Refinement: READY\nRound: 0 of 3", "missing closing line Round: <n> of 3 (n from 1 to 3)"},
		{"round four", "issue", "Refinement: READY\nRound: 4 of 3", "missing closing line Round: <n> of 3 (n from 1 to 3)"},
		{"round not last", "issue", "Refinement: READY\nRound: 1 of 3\nMore", "missing closing line Round: <n> of 3 (n from 1 to 3)"},
		{"leading blanks", "issue", "\n \r\nRefinement: READY\nGaps", "missing closing line Round: <n> of 3 (n from 1 to 3)"},
		{"review backticks", "pr", "```go\nunfinished", "unclosed fenced code block"},
		{"refinement backticks", "issue", "Refinement: READY\n```\nRound: 1 of 3", "unclosed fenced code block"},
		{"short closer", "pr", "````\n```", "unclosed fenced code block"},
		{"tilde", "pr", "~~~go\nunfinished", "unclosed fenced code block"},
		{"refinement tilde", "issue", "Refinement: READY\n~~~\nRound: 1 of 3", "unclosed fenced code block"},
		{"indented opener", "pr", "   ```\nunfinished", "unclosed fenced code block"},
		{"different closer", "pr", "```\n~~~", "unclosed fenced code block"},
		{"closer suffix", "pr", "```\n```go", "unclosed fenced code block"},
		{"indented closer", "pr", "```\n    ```", "unclosed fenced code block"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := setup(t)
			// The scan prerequisite records calls before scanning.
			t.Setenv("DEVTOOLS_PRIVATE_STRINGS_REF", "example-reference")
			write(t, filepath.Join(dir, "op"), "#!/bin/sh\necho scan >> \"$CALLS\"\nexit 1\n")
			body := tt.body
			args := []string{tt.operation, "comment", "12", "-R", "owner/repo"}
			if tt.operation == "pr" {
				body = "Reviewed head: " + reviewHead + "\n" + body
				args = []string{"pr", "review", "12", "-R", "owner/repo", "--approve", "--commit", reviewHead}
			}
			code, out, errOut := run(t, body, args...)
			require.Equal(t, 1, code, errOut)
			require.Empty(t, out)
			require.Equal(t, "gh-post: "+tt.diagnostic+"\n", errOut)
			require.NoFileExists(t, filepath.Join(dir, "calls"))
			require.NoFileExists(t, filepath.Join(dir, "payload"))
			require.NoFileExists(t, filepath.Join(dir, "state", "devtools", "gh-post.log"))
		})
	}
}

func TestCompleteBodyPosts(t *testing.T) {
	for _, tt := range []struct{ name, operation, body string }{
		{"round one", "issue", "Refinement: READY\nRound: 1 of 3"},
		{"round two", "issue", "\nRefinement: READY\r\nRound: 2 of 3\r\n \n"},
		{"round three", "issue", "Refinement: READY\nRound: 3 of 3"},
		{"literal shorter fence", "pr", "````\n```\n````"},
		{"tilde fence", "issue", "Refinement: READY\n~~~go\ntext\n~~~~\nRound: 1 of 3"},
		{"indented fence", "pr", "   ```go\ntext\n  ```   "},
		{"four space code", "pr", "    ```\ntext"},
		{"inline markers", "pr", "Text ```"},
		{"backtick in info", "pr", "```go`example"},
		{"empty comment", "issue", " \n"},
		{"ordinary issue comment", "issue", "An unrelated comment\n```"},
		{"later refinement", "issue", "Comment\nRefinement: READY\n```"},
		{"PR comment", "pr-comment", "Refinement: READY\n```"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := setup(t)
			body := tt.body
			args := []string{"issue", "comment", "12", "-R", "owner/repo"}
			switch tt.operation {
			case "pr":
				body = "Reviewed head: " + reviewHead + "\n" + body
				args = []string{"pr", "review", "12", "-R", "owner/repo", "--approve", "--commit", reviewHead}
			case "pr-comment":
				args[0] = "pr"
			}
			code, _, errOut := run(t, body, args...)
			require.Equal(t, 0, code, errOut)
			payload, err := os.ReadFile(filepath.Join(dir, "payload"))
			require.NoError(t, err)
			if tt.operation == "pr" {
				var review map[string]string
				require.NoError(t, json.Unmarshal(payload, &review))
				require.Equal(t, body, review["body"])
			} else {
				require.Equal(t, body, string(payload))
			}
			data, err := os.ReadFile(filepath.Join(dir, "state", "devtools", "gh-post.log"))
			require.NoError(t, err)
			var record auditRecord
			require.NoError(t, json.Unmarshal(data, &record))
			hash := sha256.Sum256([]byte(body))
			require.Equal(t, hex.EncodeToString(hash[:]), record.SHA256)
		})
	}
}

func TestCompleteBodyRetainsGuards(t *testing.T) {
	for _, tt := range []struct{ operation, guard string }{
		{"issue", "scan"},
		{"issue", "identity"},
		{"pr", "scan"},
		{"pr", "identity"},
	} {
		t.Run(tt.operation+"/"+tt.guard, func(t *testing.T) {
			dir := setup(t)
			body := "Refinement: READY\nRound: 1 of 3"
			args := []string{"issue", "comment", "12", "-R", "owner/repo"}
			if tt.operation == "pr" {
				body = "Reviewed head: " + reviewHead + "\nReview"
				args = []string{"pr", "review", "12", "-R", "owner/repo", "--approve", "--commit", reviewHead}
			}
			diagnostic := "gh-post: identity refused\n"
			if tt.guard == "scan" {
				t.Setenv("DEVTOOLS_PRIVATE_STRINGS", "private-marker")
				body += "\nprivate-marker\nRound: 1 of 3"
			} else {
				t.Setenv("LOGIN", "unexpected-user")
			}
			code, out, errOut := run(t, body, args...)
			require.Equal(t, 1, code, errOut)
			require.Empty(t, out)
			if tt.guard == "scan" {
				require.Contains(t, errOut, "gh-post: private-string line ")
				require.NoFileExists(t, filepath.Join(dir, "calls"))
			} else {
				require.Equal(t, diagnostic, errOut)
			}
			require.NoFileExists(t, filepath.Join(dir, "payload"))
			require.NoFileExists(t, filepath.Join(dir, "state", "devtools", "gh-post.log"))
		})
	}
}
