// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//nolint:paralleltest // These tests isolate process-wide Git configuration and identity.
package commitlint_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/commitlint"
)

const zeroOID = "0000000000000000000000000000000000000000"

func TestGitRepoIgnoresEnvironmentConfig(t *testing.T) {
	for _, name := range []string{"count", "parameters"} {
		t.Run(name, func(t *testing.T) {
			signer := filepath.Join(t.TempDir(), "missing-signer")
			t.Setenv("GIT_CONFIG_COUNT", "3")
			t.Setenv("GIT_CONFIG_KEY_0", "commit.gpg"+"sign")
			t.Setenv("GIT_CONFIG_VALUE_0", "true")
			t.Setenv("GIT_CONFIG_KEY_1", "gpg.format")
			t.Setenv("GIT_CONFIG_VALUE_1", "ssh")
			t.Setenv("GIT_CONFIG_KEY_2", "gpg.ssh.program")
			t.Setenv("GIT_CONFIG_VALUE_2", signer)
			t.Setenv("GIT_CONFIG_PARAMETERS", "")
			if name == "parameters" {
				t.Setenv("GIT_CONFIG_COUNT", "0")
				t.Setenv(
					"GIT_CONFIG_PARAMETERS",
					"'commit.gpg"+"sign=true' 'gpg.format=ssh' 'gpg.ssh.program="+signer+"'",
				)
			}

			repo := newGitRepo(t)
			makeCommit(t, repo, signedMessage("test: ignore environment config"), personIdentity(), personIdentity())
			if got := gitCommand(t, repo, "config", "--get", "commit.gpg"+"sign"); got != "false" {
				t.Fatalf("fixture signing config = %q, want false", got)
			}
		})
	}
}

func TestPrePushCleanRangePasses(t *testing.T) {
	repo := newGitRepo(t)
	base := makeCommit(t, repo, signedMessage("chore: establish base"), personIdentity(), personIdentity())
	gitCommand(t, repo, "update-ref", "refs/remotes/origin/main", base)
	tip := makeCommit(t, repo, signedMessage("fix: preserve clean pushes"), personIdentity(), personIdentity())

	status, output := runPrePush(t, repo, pushLine(tip, base))
	if status != 0 || output != "" {
		t.Fatalf("clean range: status %d, output %q", status, output)
	}
}

func TestPrePushNamesBadCommitInMiddle(t *testing.T) {
	repo := newGitRepo(t)
	base := makeCommit(t, repo, signedMessage("chore: establish base"), personIdentity(), personIdentity())
	gitCommand(t, repo, "update-ref", "refs/remotes/origin/main", base)
	bad := makeCommit(t, repo, signedMessage("not conventional"), personIdentity(), personIdentity())
	tip := makeCommit(t, repo, signedMessage("fix: finish the range"), personIdentity(), personIdentity())

	status, output := runPrePush(t, repo, pushLine(tip, base))
	if status != 1 {
		t.Fatalf("bad range status %d: %s", status, output)
	}
	want := gitCommand(t, repo, "rev-parse", "--short", bad) + " not conventional: commitlint:"
	if !strings.Contains(output, want) {
		t.Fatalf("finding does not name the bad commit and rule: %q", output)
	}
	if !strings.Contains(output, "amend or a reworded rebase through the commit skill") {
		t.Fatalf("missing repair guidance: %q", output)
	}
}

func TestPrePushNewBranchExcludesEveryRemoteRef(t *testing.T) {
	repo := newGitRepo(t)
	oldBad := makeCommit(t, repo, "old history without a signoff", personIdentity(), personIdentity())
	gitCommand(t, repo, "update-ref", "refs/remotes/upstream/main", oldBad)
	newBad := makeCommit(t, repo, "new history without a signoff", personIdentity(), personIdentity())

	status, output := runPrePush(t, repo, pushLine(newBad, zeroOID))
	if status != 1 {
		t.Fatalf("new branch status %d: %s", status, output)
	}
	if strings.Contains(output, gitCommand(t, repo, "rev-parse", "--short", oldBad)) {
		t.Fatalf("commit on another remote was checked: %q", output)
	}
	want := gitCommand(t, repo, "rev-parse", "--short", newBad) + " new history without a signoff: commitlint:"
	if !strings.Contains(output, want) {
		t.Fatalf("new commit was not named: %q", output)
	}
}

func TestPrePushSkipsDeletion(t *testing.T) {
	repo := newGitRepo(t)
	remote := makeCommit(t, repo, "bad deletion history", personIdentity(), personIdentity())

	status, output := runPrePush(t, repo, pushLine(zeroOID, remote))
	if status != 0 || output != "" {
		t.Fatalf("deletion: status %d, output %q", status, output)
	}
}

func TestPrePushRejectsAgentAuthorWithoutAssistedBy(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, ".commit-lint.toml"), "[identities]\nagents = [{email = \"agent@example.com\"}]\n")
	base := makeCommit(t, repo, signedMessage("chore: establish base"), personIdentity(), personIdentity())
	gitCommand(t, repo, "update-ref", "refs/remotes/origin/main", base)
	tip := makeCommit(t, repo, signedMessage("fix: require attribution"), agentIdentity(), personIdentity())

	status, output := runPrePush(t, repo, pushLine(tip, base))
	if status != 1 {
		t.Fatalf("agent commit status %d: %s", status, output)
	}
	want := gitCommand(t, repo, "rev-parse", "--short", tip) + " fix: require attribution: commit-trailers:"
	if !strings.Contains(output, want) || !strings.Contains(output, "requires a well-formed Assisted-by") {
		t.Fatalf("missing attributed finding: %q", output)
	}
}

func TestRangeIdentityRequiresAssistedByOnSquashMessage(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, filepath.Join(repo, ".commit-lint.toml"), "[identities]\nagents = [{email = \"agent@example.com\"}]\n")
	base := makeCommit(t, repo, signedMessage("chore: establish base"), personIdentity(), personIdentity())
	tip := makeCommit(
		t,
		repo,
		"fix: agent change\n\nAssisted-by: codex:gpt-6\nSigned-off-by: Person <person@example.com>",
		personIdentity(),
		agentIdentity(),
	)
	draft := filepath.Join(repo, "SQUASH_AGENTMSG")
	writeFile(t, draft, "fix: squash the change\n\nSigned-off-by: Person <person@example.com>\n")

	var output bytes.Buffer
	status := commitlint.Run(
		[]string{"--rules", "trailers", "--require-assisted-from", base + ".." + tip, draft},
		strings.NewReader(""),
		&output,
		repo,
		os.Getenv,
	)
	if status != 1 || !strings.Contains(output.String(), "requires a well-formed Assisted-by") {
		t.Fatalf("squash range: status %d, output %q", status, output.String())
	}
}

func TestCurrentIdentityRequiresAssistedByOnPendingCommit(t *testing.T) {
	repo := newGitRepo(t)
	const config = "[identities]\nagents = [{email = \"person@example.com\"}]\n"
	writeFile(t, filepath.Join(repo, ".commit-lint.toml"), config)

	var output bytes.Buffer
	status := commitlint.Run(
		[]string{"--rules", "trailers", "--current-identities"},
		strings.NewReader("fix: pending agent change\n\nSigned-off-by: Person <person@example.com>\n"),
		&output,
		repo,
		os.Getenv,
	)
	if status != 1 || !strings.Contains(output.String(), "requires a well-formed Assisted-by") {
		t.Fatalf("pending commit: status %d, output %q", status, output.String())
	}
}

type testIdentity struct {
	name  string
	email string
}

func personIdentity() testIdentity { return testIdentity{"Person", "person@example.com"} }

func agentIdentity() testIdentity { return testIdentity{"Agent", "agent@example.com"} }

func signedMessage(subject string) string {
	return subject + "\n\nSigned-off-by: Person <person@example.com>"
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	globalConfig := filepath.Join(t.TempDir(), "gitconfig")
	writeFile(t, globalConfig, "")
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_PARAMETERS", "")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Setenv("GIT_AUTHOR_NAME", "Person")
	t.Setenv("GIT_AUTHOR_EMAIL", "person@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Person")
	t.Setenv("GIT_COMMITTER_EMAIL", "person@example.com")
	repo := t.TempDir()
	gitCommand(t, repo, "init", "--quiet", "--initial-branch=main")
	gitCommand(t, repo, "config", "commit.gpg"+"sign", "false")
	return repo
}

func makeCommit(t *testing.T, repo, message string, author, committer testIdentity) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", "-C", repo, "commit", "--quiet", "--allow-empty", "-F", "-")
	command.Stdin = strings.NewReader(message)
	command.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+author.name,
		"GIT_AUTHOR_EMAIL="+author.email,
		"GIT_COMMITTER_NAME="+committer.name,
		"GIT_COMMITTER_EMAIL="+committer.email,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, output)
	}
	return gitCommand(t, repo, "rev-parse", "HEAD")
}

func runPrePush(t *testing.T, repo, input string) (int, string) {
	t.Helper()
	var output bytes.Buffer
	status := commitlint.Run(
		[]string{"--pre-push", "origin"},
		strings.NewReader(input),
		&output,
		repo,
		os.Getenv,
	)
	return status, output.String()
}

func pushLine(local, remote string) string {
	return fmt.Sprintf("refs/heads/topic %s refs/heads/topic %s\n", local, remote)
}

func gitCommand(t *testing.T, repo string, args ...string) string {
	t.Helper()
	//nolint:gosec // Tests pass arguments directly to Git without a shell.
	command := exec.CommandContext(t.Context(), "git", append([]string{"-C", repo}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
