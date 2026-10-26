// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

const previewGate = `package main
import("fmt";"os";"path/filepath";"strconv")
func main(){
	a:=os.Args[1:]; if len(a)!=5 || a[0]!="run" || a[1]!="--stage" || a[2]!="commit-msg" || a[3]!="--commit-msg-filename" || filepath.Base(a[4])!="COMMIT_EDITMSG" { os.Exit(99) }
	b,e:=os.ReadFile(a[4]);if e!=nil{panic(e)};if e=os.WriteFile("captured",b,0600);e!=nil{panic(e)}
	if e=os.WriteFile("preview-path",[]byte(a[4]),0600);e!=nil{panic(e)}
	fmt.Println("gate stdout");fmt.Fprintln(os.Stderr,"gate stderr");n,_:=strconv.Atoi(os.Getenv("PREVIEW_STATUS"));os.Exit(n)
}`

func previewGit(t *testing.T, args ...string) {
	t.Helper()
	c := exec.CommandContext(t.Context(), "git", args...)
	b, e := c.CombinedOutput()
	require.NoError(t, e, "%s", b)
}

func previewWrite(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

func previewBuild(t *testing.T, root, output, target string) {
	t.Helper()
	c := exec.CommandContext(t.Context(), "go", "build", "-o", output, target)
	c.Dir = root
	b, e := c.CombinedOutput()
	require.NoError(t, e, "%s", b)
}

func TestCommitPreviewParity(t *testing.T) {
	root, e := filepath.Abs("../..")
	require.NoError(t, e)
	bin := t.TempDir()
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	source := filepath.Join(bin, "gate.go")
	previewWrite(t, source, previewGate)
	previewBuild(t, root, filepath.Join(bin, "prek"+suffix), source)
	previewBuild(t, root, filepath.Join(bin, "commit-lint"+suffix), "./cmd/commit-lint")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, key := range []string{"GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM"} {
		t.Setenv(key, os.DevNull)
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	draft := "fix: preview the draft\n\nAssisted-by: codex:gpt-6\n"
	for _, name := range []string{"git", "default temp", "environment", "repository machine", "global machine", "empty environment", "empty repository", "existing", "mixed case", "gate failure", "denied git", "denied override", "invalid policy", "unset git", "behavior ignored", "worktree", "absent draft", "unreadable draft"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("PREVIEW_STATUS", "0")
			work, body := previewFixture(t, name, draft)
			want := previewResult(t, root, work, body, name, true)
			got := previewResult(t, root, work, body, name, false)
			require.Equal(t, want, got)
		})
	}
}

func previewFixture(t *testing.T, name, draft string) (string, string) {
	t.Helper()
	work := t.TempDir()
	t.Chdir(work)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(work, "config"))
	t.Setenv("TMPDIR", filepath.Join(work, "tmp"))
	require.NoError(t, os.MkdirAll(filepath.Join(work, "tmp"), 0o700))
	// Unset is observable: an explicitly empty override suppresses machine settings.
	t.Setenv("DEVTOOLS_SIGNOFF_IDENTITY", "")
	require.NoError(t, os.Unsetenv("DEVTOOLS_SIGNOFF_IDENTITY"))
	t.Setenv("PREVIEW_STATUS", "0")
	previewGit(t, "init", "-q")
	previewGit(t, "config", "user.name", "Configured Person")
	previewGit(t, "config", "user.email", "configured@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "Other Committer")
	t.Setenv("GIT_COMMITTER_EMAIL", "other@example.invalid")
	machine := filepath.Join(work, "config", "devtools")
	repo := filepath.Base(work)
	previewWrite(
		t,
		filepath.Join(machine, "devtools.conf"),
		"signoff_identity = Global Person <global@example.invalid>\n",
	)
	previewWrite(t, filepath.Join(machine, repo+".conf"), "signoff_identity = Repo Person <repo@example.invalid>\n")
	body := draft
	switch name {
	case "git", "denied git", "unset git", "behavior ignored":
		require.NoError(t, os.RemoveAll(machine))
	case "default temp":
		t.Setenv("TMPDIR", "")
	case "environment", "denied override":
		t.Setenv("DEVTOOLS_SIGNOFF_IDENTITY", "Override Person <override@example.invalid>")
	case "global machine":
		require.NoError(t, os.Remove(filepath.Join(machine, repo+".conf")))
	case "empty environment":
		t.Setenv("DEVTOOLS_SIGNOFF_IDENTITY", "")
	case "empty repository":
		previewWrite(t, filepath.Join(machine, repo+".conf"), "signoff_identity = \n")
	case "existing":
		body += "Signed-off-by: Draft Author <author@example.invalid>\nReviewed-by: Reviewer <reviewer@example.invalid>\n"
	case "mixed case":
		body += "sIgNeD-oFf-By: Draft Author <author@example.invalid>\nReviewed-by: Reviewer <reviewer@example.invalid>\n"
	case "gate failure":
		t.Setenv("PREVIEW_STATUS", "7")
	case "worktree":
		previewWrite(t, "tracked", "content")
		previewGit(t, "add", "tracked")
		previewGit(
			t,
			"-c",
			"commit.gpgSign=false",
			"-c",
			"core.hooksPath="+filepath.Join(work, "no-hooks"),
			"commit",
			"-qm",
			"initial",
		)
		linked := filepath.Join(work, "different-worktree")
		previewGit(t, "worktree", "add", "-qb", "linked", linked)
		t.Chdir(linked)
	}
	previewPolicy(t, name)
	if name != "absent draft" {
		previewWrite(t, "COMMIT_AGENTMSG", body)
	}
	if name == "unreadable draft" {
		require.NoError(t, os.Remove("COMMIT_AGENTMSG"))
		require.NoError(t, os.Mkdir("COMMIT_AGENTMSG", 0o700))
	}
	return work, body
}

func previewPolicy(t *testing.T, name string) {
	t.Helper()
	if name == "denied git" {
		previewWrite(t, ".commit-lint.toml", "[identities]\nagents = [{email = \"configured@example.invalid\"}]\n")
	}
	if name == "denied override" {
		previewWrite(t, ".commit-lint.toml", "[identities]\nagents = [{email = \"override@example.invalid\"}]\n")
	}
	if name == "invalid policy" {
		previewWrite(t, ".commit-lint.toml", "[identities]\nagents = [{name = \"Agent\"}]\n")
	}
	if name == "unset git" {
		previewGit(t, "config", "--unset", "user.name")
		previewGit(t, "config", "--unset", "user.email")
	}
	if name == "behavior ignored" {
		previewWrite(t, ".agents/agent-skills.conf", "signoff_identity = Planted Person <planted@example.invalid>\n")
	}
}

type previewObservation struct {
	code                  int
	out, stderr, captured string
}

func previewResult(t *testing.T, root, work, body, name string, frozen bool) previewObservation {
	t.Helper()
	_ = os.Remove("captured")
	_ = os.Remove("preview-path")
	var out, stderr bytes.Buffer
	var code int
	if frozen {
		// The path is the repository's frozen oracle, never fixture input.
		//nolint:gosec // Fixed repository script executed without a shell command string.
		c := exec.CommandContext(t.Context(), "bash", filepath.Join(root, ".devtools/scripts/lint-commit-msg-shell.sh"))
		c.Stdout, c.Stderr = &out, &stderr
		if err := c.Run(); err != nil {
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit)
			code = exit.ExitCode()
		}
	} else {
		code = agentrepo.Run([]string{"commit-preview"}, nil, &out, &stderr)
	}
	captured, err := os.ReadFile("captured")
	if err != nil {
		require.True(t, os.IsNotExist(err))
	}
	if name != "absent draft" && name != "unreadable draft" {
		unchanged, readErr := os.ReadFile("COMMIT_AGENTMSG")
		require.NoError(t, readErr)
		require.Equal(t, body, string(unchanged))
	}
	entries, err := os.ReadDir(filepath.Join(work, "tmp"))
	require.NoError(t, err)
	require.Empty(t, entries)
	return previewObservation{code, out.String(), stderr.String(), string(captured)}
}

func TestCommitPreviewProbe(t *testing.T) {
	t.Setenv("DEVTOOLS_SIGNOFF_IDENTITY", "denied")
	t.Chdir(t.TempDir())
	var out, stderr bytes.Buffer
	require.Zero(t, agentrepo.Run([]string{"commit-preview", "--probe"}, nil, &out, &stderr))
	require.Empty(t, out.String())
	require.Empty(t, stderr.String())
}
