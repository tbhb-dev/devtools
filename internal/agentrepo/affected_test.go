// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//go:build darwin || linux

package agentrepo_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func affectedRepo(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	gitAffected(t, "init", "-q", "-b", "main")
	gitAffected(t, "config", "user.name", "Tester")
	gitAffected(t, "config", "user.email", "tester@example.com")
	for _, dir := range []string{"scripts", "tools", "skills", "tasks", "devtools", "workflows"} {
		if err := os.MkdirAll(filepath.Join("tests", dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	bin := t.TempDir()
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$AFFECTED_LOG"
printf 'No affected tests were selected.\n'
`
	if err := os.WriteFile(filepath.Join(bin, "mise"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // The private fixture must execute as the mise stub.
	if err := os.Chmod(filepath.Join(bin, "mise"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func gitAffected(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
	return strings.TrimSpace(string(out))
}

func stageAffected(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitAffected(t, "add", "--", path)
}

func commitAffected(t *testing.T, path string) string {
	t.Helper()
	stageAffected(t, path)
	gitAffected(t, "commit", "-qm", "test: fixture")
	return gitAffected(t, "rev-parse", "HEAD")
}

func runAffected(args ...string) (int, string) {
	var out bytes.Buffer
	status := agentrepo.Run(args, nil, &out, &out)
	return status, out.String()
}

func affectedCalls(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(".git/affected-calls")
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAffectedSelection(t *testing.T) {
	for _, tt := range []struct{ path, want string }{
		{"internal/example/example.go", "run test-go\nrun test-hooks tests\n"},
		{"go.mod", "run test-go\nrun test-hooks tests\n"},
		{"packages/example/example.py", "run test-py\n"},
		{"tools/example.py", "run test-py\n"},
		{"scripts/example.sh", "run test-hooks tests/scripts\n"},
		{"tests/tools/example.bats", "run test-hooks tests/tools\n"},
		{"packages/agents/source/skills/commit/shared/scripts/commit.sh", "run check-generated-skills\nrun test-hooks tests/skills\nrun test-py\n"},
		{".agents/skills/commit/scripts/commit.sh", "run check-generated-skills\nrun test-hooks tests/skills\nrun test-py\n"},
		{".claude/skills/pr/scripts/create-pr.sh", "run check-generated-skills\nrun test-hooks tests/skills\nrun test-py\n"},
		{"README.md", "run test\n"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			t.Setenv("AFFECTED_LOG", ".git/affected-calls")
			affectedRepo(t)
			base := commitAffected(t, "internal/unselected/unselected.go")
			stageAffected(t, tt.path)
			// An unstaged Go edit must not broaden an index-only selection.
			if err := os.WriteFile(
				"internal/unselected/unselected.go", []byte("package ignored\n"), 0o600,
			); err != nil {
				t.Fatal(err)
			}
			checkAffectedSelection(t, []string{"test-affected", "--base", base, "--staged"}, tt.want)
			gitAffected(t, "commit", "-qm", "test: selected change")
			checkAffectedSelection(t, []string{"test-affected", "--base", base}, tt.want)
		})
	}
}

func TestAffectedCrossSuiteDependencies(t *testing.T) {
	for _, tt := range []struct{ path, want string }{
		{"cmd/commit-lint/main.go", "exec -- go test ./cmd/commit-lint\nrun test-hooks tests\n"},
		{"internal/commitlint/message.go", "run test-go\nrun test-hooks tests\n"},
		{"cmd/guard-markdown/run.go", "exec -- go test ./cmd/guard-markdown\nrun test-hooks tests\n"},
		{"internal/markdown/paragraph.go", "run test-go\nrun test-hooks tests\n"},
		{"internal/agentrepo/command.go", "run test-go\nrun test-hooks tests\n"},
		{"tests/skills/pr/test_helper.bash", "run test-hooks tests\n"},
		{"pyproject.toml", "run test-hooks tests\nrun test-py\n"},
		{"uv.lock", "run test-hooks tests\nrun test-py\n"},
		{"tools/skill-tokens.sh", "run test-hooks tests/scripts\nrun test-hooks tests/tools\n"},
		{"scripts/cspell-commit-msg.sh", "run test-hooks tests/scripts\nrun test-hooks tests/skills\n"},
		{"scripts/vale-commit-msg.sh", "run test-hooks tests/devtools\nrun test-hooks tests/scripts\n"},
		{"tools/lint-draft.sh", "run test-hooks tests/tasks\nrun test-hooks tests/tools\n"},
		{"tools/apm_audit.py", "run test-hooks tests/tasks\nrun test-py\n"},
		{"tools/fix_prose_replacements.py", "run test-hooks tests/tasks\nrun test-py\n"},
		{"packages/agents/source/skills/pr/shared/scripts/create-pr.sh", "run check-generated-skills\nrun test-hooks tests/skills\nrun test-py\n"},
		{"packages/agents/source/skills/merge-pr/shared/scripts/merge-policy.py", "run check-generated-skills\nrun test-hooks tests/skills\nrun test-py\n"},
		{"packages/agents/source/lib/gates.py", "run check-generated-skills\nrun test-hooks tests/skills\nrun test-hooks tests/tasks\nrun test-py\n"},
		{"packages/agents/source/skills/fix-prose/shared/scripts/check-suppressions.sh", "run check-generated-skills\nrun test-hooks tests/scripts\nrun test-hooks tests/skills\nrun test-py\n"},
		{"packages/agents/source/skills/rebase/parts/verify.md", "run check-generated-skills\nrun test-hooks tests/skills\nrun test-hooks tests/tasks\nrun test-py\n"},
		{"packages/agents/source/skills/resolve-rebase-conflicts/shared/scripts/conflict_shape.py", "run check-generated-skills\nrun test-hooks tests/skills\nrun test-hooks tests/tasks\nrun test-py\n"},
		{"packages/agents/source/skills/write-pr-description/SKILL.claude.md", "run check-generated-skills\nrun test-hooks tests/skills\nrun test-py\n"},
		{"packages/agents/claude/.apm/hooks/guard-markdown.json", "run check-generated-skills\nrun test-hooks tests/skills\nrun test-hooks tests/tasks\nrun test-py\n"},
		{".claude/skills/release/scripts/release-clone.sh", "run check-generated-skills\nrun test-hooks tests/skills\nrun test-hooks tests/tools\nrun test-py\n"},
		{".claude/skills/review-release-version/scripts/context.sh", "run check-generated-skills\nrun test-hooks tests/skills\nrun test-hooks tests/tools\nrun test-py\n"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			t.Setenv("AFFECTED_LOG", ".git/affected-calls")
			affectedRepo(t)
			base := commitAffected(t, "README.md")
			stageAffected(t, tt.path)
			checkAffectedSelection(t, []string{"test-affected", "--base", base, "--staged"}, tt.want)
			gitAffected(t, "commit", "-qm", "test: selected change")
			checkAffectedSelection(t, []string{"test-affected", "--base", base}, tt.want)
		})
	}
}

func TestAffectedSkillWithoutTestDirectory(t *testing.T) {
	t.Setenv("AFFECTED_LOG", ".git/affected-calls")
	affectedRepo(t)
	base := commitAffected(t, "README.md")
	stageAffected(t, "packages/agents/source/skills/review-commit-message/parts/body.md")
	// Shared skill checks cover manifests even when the skill has no own suite.
	if err := os.MkdirAll("tests/skills/writers", 0o700); err != nil {
		t.Fatal(err)
	}
	checkAffectedSelection(t, []string{"test-affected", "--base", base, "--staged"},
		"run check-generated-skills\nrun test-hooks tests/skills\nrun test-py\n")
	gitAffected(t, "commit", "-qm", "test: selected change")
	checkAffectedSelection(t, []string{"test-affected", "--base", base},
		"run check-generated-skills\nrun test-hooks tests/skills\nrun test-py\n")
}

func TestAffectedOwnedInputs(t *testing.T) {
	for _, tt := range []struct{ path, want string }{
		{"internal/commitlint/testdata/legacy.json", "run test-go\nrun test-hooks tests\n"},
		{"internal/signing/units/signing-agent.service.tmpl", "run test-go\nrun test-hooks tests\n"},
		{"internal/signing/units/dev.tbhb.devtools.signing.plist.tmpl", "run test-go\nrun test-hooks tests\n"},
		{"internal/config/testdata/legacy-config.sh", "run test-go\nrun test-hooks tests\n"},
		{"tests/workflows/fixtures/govulncheck.sarif", "run test-hooks tests/workflows\n"},
		{"tests/workflows/fixtures/rust-msrv/member/Cargo.toml", "run test-hooks tests/workflows\n"},
		{"tests/tasks/fixtures/pins/mise.toml", "run test-hooks tests/tasks\n"},
		{"tests/tasks/fixtures/rust/Cargo.toml", "run test-hooks tests/tasks\n"},
		{".github/pull_request_template.md", "run test\n"},
		{".github/workflows/security.yml", "run test\n"},
		{"cog.toml", "run test\n"},
		{"packages/devtools/pyproject.toml", "run test\n"},
		{"packages/devtools/data.json", "run test\n"},
		{"AGENTS.md", "run test\n"},
		{"mise.lock", "run test\n"},
		{".vale/styles/example.yml", "run test\n"},
		{".pre-commit-hooks.yaml", "run test\n"},
		{"unknown/new-input.json", "run test\n"},
		{"unknown/helper.sh", "run test\n"},
		{"tests/nonexistent/deleted.bats", "run test\n"},
		{"tests/git-environment.bats", "run test-hooks tests\n"},
		{"docs/guide.md", ""},
	} {
		for _, staged := range []bool{true, false} {
			mode := "committed"
			if staged {
				mode = "staged"
			}
			t.Run(tt.path+"/"+mode, func(t *testing.T) {
				t.Setenv("AFFECTED_LOG", ".git/affected-calls")
				checkAffectedInput(t, tt.path, tt.want, staged)
			})
		}
	}
}

func checkAffectedInput(t *testing.T, path, want string, staged bool) {
	t.Helper()
	affectedRepo(t)
	// Non-Go inputs belong to the closest ancestor containing Go sources.
	for _, pkg := range []string{"commitlint", "signing", "config"} {
		stageAffected(t, "internal/"+pkg+"/package.go")
	}
	base := commitAffected(t, "README.md")
	stageAffected(t, path)
	if strings.HasPrefix(path, "tests/nonexistent/") {
		gitAffected(t, "commit", "-qm", "test: fixture to delete")
		base = gitAffected(t, "rev-parse", "HEAD")
		gitAffected(t, "rm", "-qr", "tests/nonexistent")
	}
	args := []string{"test-affected", "--base", base}
	if staged {
		args = append(args, "--staged")
	} else {
		gitAffected(t, "commit", "-qm", "test: selected change")
	}
	checkAffectedSelection(t, args, want)
}

func TestAffectedVendoredPackages(t *testing.T) {
	for _, path := range []string{
		"vendor/example.com/dep/other.go", "vendor/example.com/dep/LICENSE", "vendor/modules.txt",
	} {
		t.Run(path, func(t *testing.T) {
			affectedRepo(t)
			t.Setenv("AFFECTED_LOG", ".git/affected-calls")
			// The vendored directory holds Go sources, so only the module suite covers it.
			stageAffected(t, "vendor/example.com/dep/dep.go")
			base := commitAffected(t, "README.md")
			stageAffected(t, path)
			checkAffectedSelection(t, []string{"test-affected", "--base", base, "--staged"},
				"run test-go\nrun test-hooks tests\n")
			gitAffected(t, "commit", "-qm", "test: vendored change")
			checkAffectedSelection(t, []string{"test-affected", "--base", base},
				"run test-go\nrun test-hooks tests\n")
		})
	}
}

func TestAffectedSharedInternalPackages(t *testing.T) {
	// Importing packages such as signing, reviewer, agentrepo and guard-markdown
	// carry their own Go unit tests, so a shared internal package selects them all.
	for _, pkg := range []string{
		"buildmeta", "checks", "config", "credentials", "ghapp", "hookio", "markdown", "review",
	} {
		t.Run(pkg, func(t *testing.T) {
			affectedRepo(t)
			t.Setenv("AFFECTED_LOG", ".git/affected-calls")
			base := commitAffected(t, "README.md")
			stageAffected(t, "internal/"+pkg+"/"+pkg+".go")
			checkAffectedSelection(t, []string{"test-affected", "--base", base, "--staged"},
				"run test-go\nrun test-hooks tests\n")
			gitAffected(t, "commit", "-qm", "test: shared package change")
			checkAffectedSelection(t, []string{"test-affected", "--base", base},
				"run test-go\nrun test-hooks tests\n")
		})
	}
}

func TestAffectedConfiguration(t *testing.T) {
	for _, path := range []string{
		"mise.toml", ".config/mise/conf.d/tasks.toml", ".devtools/tasks/go.toml",
		".config/mise/validate.py", ".devtools/scripts/lint-source-files.py",
	} {
		t.Run(path, func(t *testing.T) {
			affectedRepo(t)
			t.Setenv("AFFECTED_LOG", ".git/affected-calls")
			base := commitAffected(t, "README.md")
			stageAffected(t, path)
			checkAffectedSelection(t, []string{"test-affected", "--base", base, "--staged"}, "run test\n")
			gitAffected(t, "commit", "-qm", "test: configuration")
			checkAffectedSelection(t, []string{"test-affected", "--base", base}, "run test\n")
		})
	}
}

func TestAffectedDeduplication(t *testing.T) {
	affectedRepo(t)
	t.Setenv("AFFECTED_LOG", ".git/affected-calls")
	base := commitAffected(t, "README.md")
	stageAffected(t, "tests/test_helper.bash")
	stageAffected(t, "tests/tools/example.bats")
	checkAffectedSelection(t, []string{"test-affected", "--base", base, "--staged"}, "run test-hooks tests\n")
	stageAffected(t, "internal/example/example.go")
	stageAffected(t, "go.mod")
	checkAffectedSelection(t, []string{"test-affected", "--base", base, "--staged"},
		"run test-go\nrun test-hooks tests\n")
	stageAffected(t, "unowned.json")
	checkAffectedSelection(t, []string{"test-affected", "--base", base, "--staged"}, "run test\n")
}

func TestAffectedMissingSuite(t *testing.T) {
	affectedRepo(t)
	t.Setenv("AFFECTED_LOG", ".git/affected-calls")
	base := commitAffected(t, "README.md")
	stageAffected(t, "packages/agents/source/skills/commit/parts/commit.md")
	if err := os.RemoveAll("tests/skills"); err != nil {
		t.Fatal(err)
	}
	checkAffectedSelection(t, []string{"test-affected", "--base", base, "--staged"}, "run test\n")
}

func TestAffectedMissingBase(t *testing.T) {
	t.Parallel()
	cmd, _ := agentrepo.NewCommand()
	cmd.SetArgs([]string{"test-affected"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	if err == nil || err.Error() != "affected tests: --base is required" {
		t.Fatalf("missing base: %v", err)
	}
}

func TestAffectedGitFailure(t *testing.T) {
	for _, invalidRevision := range []bool{true, false} {
		t.Run(map[bool]string{true: "invalid revision", false: "diff failure"}[invalidRevision], func(t *testing.T) {
			affectedRepo(t)
			t.Setenv("AFFECTED_LOG", ".git/affected-calls")
			base := commitAffected(t, "README.md")
			if invalidRevision {
				base = "not-a-revision"
			} else {
				// A corrupt index makes diff fail even with a valid base.
				if err := os.WriteFile(".git/index", []byte("invalid index"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			status, out := runAffected("test-affected", "--base", base, "--staged")
			if status == 0 || !strings.Contains(out, "affected tests: git diff") || affectedCalls(t) != "" {
				t.Fatalf("status=%d out=%s calls=%q", status, out, affectedCalls(t))
			}
		})
	}
}

func TestAffectedMiseFailure(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "failing", true: "missing"}[missing], func(t *testing.T) {
			affectedRepo(t)
			t.Setenv("AFFECTED_LOG", ".git/affected-calls")
			base := commitAffected(t, "README.md")
			stageAffected(t, "tools/example.py")
			affectedFailurePath(t, missing)
			cmd, _ := agentrepo.NewCommand()
			cmd.SetArgs([]string{"test-affected", "--base", base, "--staged"})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "affected tests: mise:") {
				t.Fatalf("missing=%t err=%v out=%s", missing, err, &out)
			}
			var exitErr *exec.ExitError
			if missing {
				if !errors.Is(err, exec.ErrNotFound) {
					t.Fatalf("missing mise: %v", err)
				}
			} else if !errors.As(err, &exitErr) || exitErr.ExitCode() != 42 {
				t.Fatalf("failing mise: %v", err)
			}
		})
	}
}

func affectedFailurePath(t *testing.T, missing bool) {
	t.Helper()
	bin := t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(git, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	if !missing {
		//nolint:gosec // The private fixture must execute as the failing mise stub.
		if err = os.WriteFile(filepath.Join(bin, "mise"), []byte("#!/bin/sh\nexit 42\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
}

func checkAffectedSelection(t *testing.T, args []string, want string) {
	t.Helper()
	if err := os.Remove(".git/affected-calls"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	status, out := runAffected(args...)
	if status != 0 || affectedCalls(t) != want {
		t.Fatalf("AC6: args=%v status=%d out=%s calls=%q want=%q", args, status, out, affectedCalls(t), want)
	}
	if want == "" && !strings.Contains(out, "No affected tests were selected.") {
		t.Fatal("AC4: missing empty selection message")
	}
}
