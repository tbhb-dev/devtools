// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

const contractFixtures = "../../tests/fixtures/contract"

func contractRun(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	code := agentrepo.Run(append([]string{"contract"}, args...), nil, &out, &stderr)
	return code, out.String() + stderr.String()
}

func contractGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid",
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// contractWrite writes a file under the test's temporary directory, creating
// its parent directory first.
func contractWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // Tests write inside directories they created under t.TempDir.
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// contractInstall puts a fixture contract in the directory and renders its notes.
func contractInstall(t *testing.T, dir, contract string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(contractFixtures, contract))
	if err != nil {
		t.Fatal(err)
	}
	contractWrite(t, filepath.Join(dir, ".devtools", "consumer-contract.toml"), string(data))
	if err = os.MkdirAll(filepath.Join(dir, "docs"), 0o750); err != nil {
		t.Fatal(err)
	}
	if code, out := contractRun(t, "--repo", dir, "render"); code != 0 {
		t.Fatalf("render: %s", out)
	}
}

// contractRelease replaces the repository's tree with a fixture reader and contract,
// renders the notes the way a release does, and commits, tagging when asked.
func contractRelease(t *testing.T, dir, reader, contract, tag string) {
	t.Helper()
	for _, entry := range []string{".claude", ".devtools", "docs"} {
		if err := os.RemoveAll(filepath.Join(dir, entry)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.CopyFS(dir, os.DirFS(filepath.Join(contractFixtures, reader))); err != nil {
		t.Fatal(err)
	}
	if contract != "" {
		contractInstall(t, dir, contract)
	}
	contractGit(t, dir, "add", "-A")
	contractGit(t, dir, "commit", "-q", "--allow-empty", "-m", "release")
	if tag != "" {
		contractGit(t, dir, "tag", tag)
	}
}

func TestContractRenderWritesAndChecksTheNotes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	contractGit(t, dir, "init", "-q", "-b", "main")
	contractRelease(t, dir, "reader-v1", "contract-baseline.toml", "")
	if code, out := contractRun(t, "--repo", dir, "render", "--check"); code != 0 {
		t.Fatalf("fresh notes: %s", out)
	}
	notes := filepath.Join(dir, "docs", "consumer-transitions.md")
	contractWrite(t, notes, "stale\n")
	if code, out := contractRun(
		t,
		"--repo",
		dir,
		"render",
		"--check",
	); code != 1 ||
		!strings.Contains(out, "generate-consumer-notes") {
		t.Fatalf("stale notes: %d %s", code, out)
	}
	if code, out := contractRun(t, "--repo", dir, "render"); code != 0 {
		t.Fatalf("rewrite: %s", out)
	}
	if code, out := contractRun(t, "--repo", dir, "render", "--check"); code != 0 {
		t.Fatalf("rewritten notes: %s", out)
	}
}

func TestContractRenderRefusesAnUnreadableContract(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if code, _ := contractRun(t, "--repo", dir, "render"); code != 1 {
		t.Fatalf("missing: %d", code)
	}
	contractWrite(t, filepath.Join(dir, ".devtools", "consumer-contract.toml"), "schema_version = 7\n")
	if code, out := contractRun(t, "--repo", dir, "render"); code != 1 || !strings.Contains(out, "unsupported") {
		t.Fatalf("schema: %d %s", code, out)
	}
}

func TestContractValidateJudgesTheCandidate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	contractGit(t, dir, "init", "-q", "-b", "main")
	contractRelease(t, dir, "reader-v1", "", "v1.0.0")
	contractRelease(t, dir, "reader-v1", "contract-baseline.toml", "")
	code, out := contractRun(t, "--repo", dir, "validate")
	if code != 0 || !strings.Contains(out, "valid against v1.0.0 (0 transitions declared)") {
		t.Fatalf("baseline: %d %s", code, out)
	}
	contractRelease(t, dir, "reader-v1", "contract-baseline.toml", "v1.1.0")
	contractRelease(t, dir, "reader-v3", "contract-require-undeclared.toml", "")
	code, out = contractRun(t, "--repo", dir, "validate")
	if code != 1 || !strings.Contains(out, "gamma") {
		t.Fatalf("undeclared: %d %s", code, out)
	}
}

func TestContractPlanApplyAndVerifyMoveAConsumerForward(t *testing.T) {
	t.Parallel()
	remote := t.TempDir()
	contractGit(t, remote, "init", "-q", "-b", "main")
	contractRelease(t, remote, "reader-v1", "contract-baseline.toml", "v1.1.0")
	contractRelease(t, remote, "reader-v2", "contract-bridge.toml", "v1.2.0")
	contractRelease(t, remote, "reader-v3", "contract-require.toml", "v1.3.0")
	consumer := t.TempDir()
	contractGit(t, consumer, "clone", "-q", remote, ".")
	contractGit(t, consumer, "checkout", "-q", "v1.2.0")
	conf := filepath.Join(consumer, ".agents", "agent-skills.conf")
	contractWrite(t, conf, "alpha = x\n")
	state := t.TempDir()
	code, out := contractRun(t, "--repo", consumer, "plan", "--remote", remote, "--tag", "v1.3.0", "--state", state)
	if code != 0 || !strings.Contains(out, "transition: required-key-added") {
		t.Fatalf("plan: %d %s", code, out)
	}
	if code, out = contractRun(
		t,
		"--repo",
		consumer,
		"apply",
		"--state",
		state,
	); code != 0 ||
		!strings.Contains(out, "updated") {
		t.Fatalf("apply: %d %s", code, out)
	}
	if code, out = contractRun(t, "--repo", consumer, "verify", "--state", state); code != 0 {
		t.Fatalf("verify: %d %s", code, out)
	}
	if got, err := os.ReadFile(conf); err != nil || string(got) != "alpha = x\ngamma = on\n" {
		t.Fatalf("conf %q %v", got, err)
	}
	// A second pass finds the consumer already satisfied.
	state = t.TempDir()
	if code, out = contractRun(
		t,
		"--repo",
		consumer,
		"plan",
		"--remote",
		remote,
		"--tag",
		"v1.3.0",
		"--state",
		state,
	); code != 0 ||
		!strings.Contains(out, "no consumer transition applies") {
		t.Fatalf("second plan: %d %s", code, out)
	}
	if code, out = contractRun(t, "--repo", consumer, "apply", "--state", state); code != 0 || out != "" {
		t.Fatalf("second apply: %d %s", code, out)
	}
}

func TestContractPlanRefusesAConsumerBeforeTheBaseline(t *testing.T) {
	t.Parallel()
	remote := t.TempDir()
	contractGit(t, remote, "init", "-q", "-b", "main")
	contractRelease(t, remote, "reader-v1", "", "v1.0.0")
	contractRelease(t, remote, "reader-v1", "contract-baseline.toml", "v1.1.0")
	contractRelease(t, remote, "reader-v2", "contract-bridge.toml", "v1.2.0")
	consumer := t.TempDir()
	contractGit(t, consumer, "clone", "-q", remote, ".")
	contractGit(t, consumer, "checkout", "-q", "v1.0.0")
	code, out := contractRun(
		t,
		"--repo",
		consumer,
		"plan",
		"--remote",
		remote,
		"--tag",
		"v1.2.0",
		"--state",
		t.TempDir(),
	)
	if code != 1 || !strings.Contains(out, "v1.1.0") {
		t.Fatalf("%d %s", code, out)
	}
	if code, _ = contractRun(t, "--repo", consumer, "verify", "--state", t.TempDir()); code != 1 {
		t.Fatalf("verify without a plan: %d", code)
	}
	if code, _ = contractRun(t, "--repo", consumer, "plan"); code == 0 {
		t.Fatal("plan without flags passed")
	}
}
