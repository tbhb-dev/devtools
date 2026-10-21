// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommands(t *testing.T) {
	t.Parallel()
	tests := []struct{ args, want []string }{
		{
			[]string{"issue", "create", "--title", "title"},
			[]string{"issue", "create", "--repo", "owner/repo", "--title", "title", "--body-file", "-"},
		},
		{
			[]string{"issue", "comment", "12"},
			[]string{"issue", "comment", "12", "--repo", "owner/repo", "--body-file", "-"},
		},
		{
			[]string{"issue", "edit", "12", "--title=title"},
			[]string{"issue", "edit", "12", "--repo", "owner/repo", "--title", "title", "--body-file", "-"},
		},
		{
			[]string{"pr", "create", "--title", "title", "--draft", "--head", "topic", "--base", "main"},
			[]string{
				"pr",
				"create",
				"--repo",
				"owner/repo",
				"--title",
				"title",
				"--draft",
				"--head",
				"topic",
				"--base",
				"main",
				"--body-file",
				"-",
			},
		},
		{
			[]string{"pr", "edit", "12", "--add-label", "bug"},
			[]string{"pr", "edit", "12", "--repo", "owner/repo", "--add-label", "bug", "--body-file", "-"},
		},
		{
			[]string{"pr", "comment", "12", "--edit-last"},
			[]string{"pr", "comment", "12", "--repo", "owner/repo", "--edit-last", "--body-file", "-"},
		},
		{
			[]string{"pr", "review", "12", "--approve"},
			[]string{"pr", "review", "12", "--repo", "owner/repo", "--approve", "--body-file", "-"},
		},
		{
			[]string{"release", "create", "v1", "--title", "name", "--verify-tag"},
			[]string{
				"release",
				"create",
				"v1",
				"--repo",
				"owner/repo",
				"--title",
				"name",
				"--verify-tag",
				"--notes-file",
				"-",
			},
		},
		{
			[]string{"release", "edit", "v1", "--prerelease"},
			[]string{"release", "edit", "v1", "--repo", "owner/repo", "--prerelease", "--notes-file", "-"},
		},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			t.Parallel()
			r, err := parse(append(tt.args, "-R", "owner/repo", "--body-file", "-"))
			require.NoError(t, err)
			args, input, err := r.command([]byte("body\n"))
			require.NoError(t, err)
			require.Equal(t, tt.want, args)
			require.Equal(t, "body\n", string(input))
		})
	}
}

func TestCommentEdits(t *testing.T) {
	t.Parallel()
	for kind, endpoint := range map[string]string{"issue": "issues", "review": "pulls"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			r, err := parse([]string{"comment", "edit", "42", "--kind", kind, "--repo=owner/repo"})
			require.NoError(t, err)
			args, input, err := r.command([]byte("body\n\"quoted\""))
			require.NoError(t, err)
			require.Equal(
				t,
				[]string{
					"api",
					"--method",
					"PATCH",
					"repos/owner/repo/" + endpoint + "/comments/42",
					"--input",
					"-",
					"--jq",
					".html_url",
				},
				args,
			)
			var payload map[string]string
			require.NoError(t, json.Unmarshal(input, &payload))
			require.Equal(t, map[string]string{"body": "body\n\"quoted\""}, payload)
		})
	}
}

func TestRejectArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{},
		{"pr"},
		{"repo", "create"},
		{"pr", "comment"},
		{"issue", "create"},
		{"pr", "create", "extra", "--title", "x"},
		{"pr", "comment", "1", "extra"},
		{"pr", "comment", "1", "--body", "text"},
		{"issue", "comment", "1", "-b", "text"},
		{"pr", "comment", "1", "-F", "file"},
		{"pr", "comment", "1", "--body=text"},
		{"pr", "comment", "1", "--template", "file"},
		{"pr", "comment", "1", "--web"},
		{"pr", "comment", "1", "--unknown=text"},
		{"pr", "comment", "1", "--repo"},
		{"pr", "comment", "1", "--body-file", "-bad"},
		{"pr", "create", "--title", "--fill"},
		{"pr", "comment", "1", "--repo", "a/b", "-R", "c/d"},
		{"pr", "comment", "1", "--body-file", "a", "--body-file", "b"},
		{"pr", "review", "1"},
		{"pr", "review", "1", "--approve", "--comment"},
		{"pr", "review", "1", "--approve=true"},
		{"pr", "edit", "https://github.com/other/repo/pull/1"},
		{"comment", "edit", "1"},
		{"comment", "edit", "1", "--kind", "other"},
	} {
		t.Run(
			strings.Join(args, " "),
			func(t *testing.T) { t.Parallel(); _, err := parse(args); require.ErrorIs(t, err, errUsage) },
		)
	}
}
