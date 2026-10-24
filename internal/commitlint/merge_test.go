// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//nolint:paralleltest // These tests isolate process-wide Git configuration and identity.
package commitlint_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/commitlint"
)

func TestCommitMessageMergeSubjects(t *testing.T) {
	for _, subject := range []string{
		"Merge branch 'main'",
		"Merge branch 'main' into topic",
		"Merge branch 'main' of https://example.com/repo",
		"Merge branch 'main' of https://example.com/repo into topic",
		"Merge remote-tracking branch 'origin/main'",
		"Merge remote-tracking branch 'origin/main' into topic",
		"Merge commit '0123456789abcdef'",
		"Merge commit '0123456789abcdef' into topic",
	} {
		t.Run(subject, func(t *testing.T) {
			repo := newGitRepo(t)
			draft := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
			writeFile(t, draft, signedMessage(subject))
			for _, merge := range []bool{false, true} {
				wantStatus := 1
				wantOutput := "commitlint: subject must follow Conventional Commits: <type>(<scope>)?(!)?: <description>\n"
				if merge {
					writeFile(t, filepath.Join(repo, ".git", "MERGE_HEAD"), strings.Repeat("a", 40)+"\n")
					wantStatus, wantOutput = 0, ""
				}
				var output bytes.Buffer
				status := commitlint.Run([]string{draft}, strings.NewReader(""), &output, repo, os.Getenv)
				if status != wantStatus || output.String() != wantOutput {
					t.Fatalf(
						"merge %t: got (%d, %q), want (%d, %q)",
						merge,
						status,
						output.String(),
						wantStatus,
						wantOutput,
					)
				}
			}
		})
	}
}

func TestMergeSubjectStillChecksOtherRules(t *testing.T) {
	for _, tt := range []struct {
		name, message, finding string
	}{
		{"custom subject", signedMessage("merge the branches"), "subject must follow Conventional Commits"},
		{"prefix", signedMessage("prefix Merge branch 'main'"), "subject must follow Conventional Commits"},
		{"suffix", signedMessage("Merge branch 'main' with edits"), "subject must follow Conventional Commits"},
		{"missing quote", signedMessage("Merge branch 'main"), "subject must follow Conventional Commits"},
		{"empty branch", signedMessage("Merge branch ''"), "subject must follow Conventional Commits"},
		{"invalid commit", signedMessage("Merge commit 'not-a-sha'"), "subject must follow Conventional Commits"},
		{"empty target", signedMessage("Merge branch 'main' into "), "subject must follow Conventional Commits"},
		{"unknown type", signedMessage("unknown: merge branches"), "unknown type"},
		{"signoff", "Merge branch 'main'", "missing required Signed-off-by"},
		{"attribution", "Merge branch 'main'\n\nAssisted-by: invalid\nSigned-off-by: Person <person@example.com>", "malformed Assisted-by"},
		{"body", signedMessage("Merge branch 'main'") + "\n\n" + strings.Repeat("a", 73), "body line"},
		{"conventional", signedMessage("fix: merge the branches"), ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newGitRepo(t)
			writeFile(t, filepath.Join(repo, ".git", "MERGE_HEAD"), strings.Repeat("a", 40)+"\n")
			var output bytes.Buffer
			status := commitlint.Run(nil, strings.NewReader(tt.message), &output, repo, os.Getenv)
			if tt.finding == "" {
				if status != 0 || output.Len() != 0 {
					t.Fatalf("status %d, output %q", status, output.String())
				}
			} else if status != 1 || !strings.Contains(output.String(), tt.finding) {
				t.Fatalf("status %d, output %q; want %q", status, output.String(), tt.finding)
			}
		})
	}
}

func TestCommitMessageMergeInWorktree(t *testing.T) {
	repo := newGitRepo(t)
	makeCommit(t, repo, signedMessage("chore: establish base"), personIdentity(), personIdentity())
	worktree := filepath.Join(t.TempDir(), "worktree")
	gitCommand(t, repo, "worktree", "add", "--quiet", "-b", "topic", worktree)
	mergeHead := gitCommand(t, worktree, "rev-parse", "--git-path", "MERGE_HEAD")
	writeFile(t, mergeHead, strings.Repeat("a", 40)+"\n")
	subdir := filepath.Join(worktree, "subdir")
	if err := os.Mkdir(subdir, 0o750); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	status := commitlint.Run(nil, strings.NewReader(signedMessage("Merge branch 'main'")), &output, subdir, os.Getenv)
	if status != 0 || output.Len() != 0 {
		t.Fatalf("worktree merge: status %d, output %q", status, output.String())
	}
}

func TestPrePushMergeSubjects(t *testing.T) {
	for _, tt := range []struct {
		name, message, finding string
		merge                  bool
	}{
		{"branch", signedMessage("Merge branch 'main' into topic"), "", true},
		{"remote", signedMessage("Merge remote-tracking branch 'origin/main'"), "", true},
		{"commit", signedMessage("Merge commit '0123456789abcdef'"), "", true},
		{"non-merge", signedMessage("Merge branch 'main'"), "subject must follow Conventional Commits", false},
		{"custom", signedMessage("merge the branches"), "subject must follow Conventional Commits", true},
		{"signoff", "Merge branch 'main'", "missing required Signed-off-by", true},
		{"attribution", "Merge branch 'main'\n\nAssisted-by: invalid\nSigned-off-by: Person <person@example.com>", "malformed Assisted-by", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newGitRepo(t)
			base := makeCommit(t, repo, signedMessage("chore: establish base"), personIdentity(), personIdentity())
			gitCommand(t, repo, "update-ref", "refs/remotes/origin/main", base)
			tip := makeCommit(t, repo, tt.message, personIdentity(), personIdentity())
			if tt.merge {
				tree := gitCommand(t, repo, "rev-parse", "HEAD^{tree}")
				tip = gitCommand(t, repo, "commit-tree", tree, "-p", base, "-p", tip, "-m", tt.message)
				// Exclude the fixture parent so only the merge message is checked.
				gitCommand(t, repo, "update-ref", "refs/remotes/origin/side", "HEAD")
			}
			status, output := runPrePush(t, repo, pushLine(tip, base))
			if tt.finding == "" {
				if status != 0 || output != "" {
					t.Fatalf("status %d, output %q", status, output)
				}
			} else if status != 1 || !strings.Contains(output, tt.finding) {
				t.Fatalf("status %d, output %q; want %q", status, output, tt.finding)
			}
		})
	}
}
