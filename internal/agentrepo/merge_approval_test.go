// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestMergeApproval(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, input string
		extra       []string
		want        int
	}{
		{"approval", `{"reviews":[{"author":{"login":"reviewer","__typename":"Bot"},"state":"APPROVED","commit":{"oid":"head"}}]}`, nil, 0},
		{"no approval", `{"reviews":[]}`, nil, 1},
		{"typed mismatch", `{"reviews":[{"author":{"login":"reviewer"},"state":"APPROVED","commit":{"oid":"head"}}]}`, []string{"--author-type", "Bot"}, 1},
		{"empty login", `{"reviews":[]}`, []string{"--login", ""}, 1},
		{"invalid JSON", `broken`, nil, 5},
		{"invalid type", `{"reviews":[]}`, []string{"--author-type", "App"}, 5},
		{"empty head", `{"reviews":[]}`, []string{"--head", ""}, 5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			file := filepath.Join(t.TempDir(), "reviews.json")
			if err := os.WriteFile(file, []byte(tt.input), 0o600); err != nil {
				t.Fatal(err)
			}
			args := make([]string, 0, 8+len(tt.extra))
			args = append(args, "merge", "approval", "--file", file, "--login", "reviewer", "--head", "head")
			var out, stderr bytes.Buffer
			got := agentrepo.Run(append(args, tt.extra...), nil, &out, &stderr)
			if got != tt.want || out.Len() != 0 {
				t.Fatalf("code %d, stdout %s, stderr %s", got, &out, &stderr)
			}
			data, err := os.ReadFile(file)
			if err != nil || string(data) != tt.input {
				t.Fatal("snapshot changed", err)
			}
		})
	}
}

func TestMergeApprovalInvalidArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"merge", "approval"},
		{"merge", "approval", "--file", "/nonexistent/reviews.json", "--login", "reviewer", "--head", "head"},
		{"merge", "approval", "--unknown"},
		{"merge", "approval", "unexpected"},
	} {
		var out bytes.Buffer
		if got := agentrepo.Run(args, nil, &out, &out); got != 5 {
			t.Fatalf("%v: %d", args, got)
		}
	}
}
