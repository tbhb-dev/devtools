// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/consumercontract"
)

const fixtures = "../../tests/fixtures/contract"

// gitEnv is a Git environment that ignores host configuration and identity.
func gitEnv(t *testing.T) []string {
	t.Helper()
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid",
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	if err := os.CopyFS(to, os.DirFS(from)); err != nil {
		t.Fatal(err)
	}
}

// writeFile writes a file under the test's temporary directory, creating its
// parent directory first.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // Tests write inside directories they created under t.TempDir.
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeTree replaces the tracked content of dir with a fixture reader and,
// when contract is set, the fixture contract and its rendered notes.
func writeTree(t *testing.T, dir, reader, contract string) {
	t.Helper()
	for _, entry := range []string{".claude", ".devtools", "docs"} {
		if err := os.RemoveAll(filepath.Join(dir, entry)); err != nil {
			t.Fatal(err)
		}
	}
	copyTree(t, filepath.Join(fixtures, reader), dir)
	if contract == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(fixtures, contract))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := consumercontract.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		consumercontract.ContractPath: string(data),
		consumercontract.NotesPath:    consumercontract.Render(doc),
	} {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(path)), content)
	}
}

type release struct{ tag, reader, contract string }

// releases builds a repository with one commit per release, each tagged.
func releases(t *testing.T, steps ...release) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	for _, step := range steps {
		writeTree(t, repo, step.reader, step.contract)
		git(t, repo, "add", "-A")
		git(t, repo, "commit", "-q", "-m", "release "+step.tag)
		git(t, repo, "tag", step.tag)
	}
	return repo
}

func requireProbeTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"bash", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
}

func parseFixture(t *testing.T, name string) *consumercontract.Document {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtures, name))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := consumercontract.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
