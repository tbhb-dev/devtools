//go:build unix

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package skills_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/skills"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "git", "init", "--quiet", root)
	data, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", data)
	return root
}

func snapshot(t *testing.T, root string) string {
	t.Helper()
	tree, openErr := os.OpenRoot(root)
	require.NoError(t, openErr)
	defer tree.Close()
	var state strings.Builder
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk snapshot: %w", err)
		}
		if entry.IsDir() {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("snapshot path: %w", relErr)
		}
		var data []byte
		var readErr error
		if entry.Type()&os.ModeSymlink != 0 {
			var target string
			target, readErr = tree.Readlink(relative)
			data = []byte(target)
		} else {
			data, readErr = tree.ReadFile(relative)
		}
		if readErr != nil {
			return fmt.Errorf("read snapshot: %w", readErr)
		}
		_, _ = fmt.Fprintf(&state, "%s %x\n", path, sha256.Sum256(data))
		return nil
	}))
	return state.String()
}

// shellTimeout expires well before any package timeout, so a hung shell is killed by its own context.
const shellTimeout = 30 * time.Second

// runGrouped runs a command in its own process group under a deadline. The group is killed on
// expiry and again after the command returns, so no descendant keeps running after the call returns.
func runGrouped(
	t *testing.T, timeout time.Duration, dir string, env []string, name string, args ...string,
) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	// Descendants still running after the command exits share its group.
	// With no live process left in the group, macOS refuses the signal as not permitted.
	// The signal goes out before any error handling, so the wait-delay error and a failed assertion both reach it.
	if cmd.Process != nil {
		killErr := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if killErr != nil && !errors.Is(killErr, syscall.ESRCH) && !errors.Is(killErr, syscall.EPERM) {
			t.Errorf("kill process group: %v", killErr)
		}
	}
	code := cmd.ProcessState.ExitCode()
	if runErr != nil && !errors.Is(runErr, exec.ErrWaitDelay) {
		var exit *exec.ExitError
		require.ErrorAs(t, runErr, &exit)
	}
	return stdout.String(), stderr.String(), code
}

func TestRunGroupedKillsDescendants(t *testing.T) {
	t.Parallel()
	pidFile := filepath.Join(t.TempDir(), "pid")
	_, _, code := runGrouped(t, 500*time.Millisecond, t.TempDir(), nil,
		"bash", "-c", `sleep 300 & echo $! > "$0"; wait`, pidFile)
	require.NotZero(t, code, "the deadline kills the shell")
	data, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
	}, 5*time.Second, 50*time.Millisecond, "the background sleep must not survive")
}

func TestRunGroupedKillsAfterWaitDelay(t *testing.T) {
	t.Parallel()
	pidFile := filepath.Join(t.TempDir(), "pid")
	_, _, code := runGrouped(t, shellTimeout, t.TempDir(), nil,
		"bash", "-c", `sleep 300 & echo $! > "$0"`, pidFile)
	require.Zero(t, code, "the shell itself exits cleanly while its child holds the output pipes")
	data, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
	}, 5*time.Second, 50*time.Millisecond, "the background sleep must not survive the wait-delay error")
}

func parity(t *testing.T, root string, targets []string, traceback bool) {
	t.Helper()
	script, err := filepath.Abs("../../tools/skill-limits-legacy.sh")
	require.NoError(t, err)
	before := snapshot(t, root)
	locales := []string{"C"}
	available, localeErr := exec.CommandContext(t.Context(), "locale", "-a").Output()
	require.NoError(t, localeErr)
	for locale := range strings.FieldsSeq(string(available)) {
		if strings.Contains(strings.ToLower(locale), "utf") {
			locales = append(locales, locale)
			break
		}
	}
	for _, locale := range locales {
		stdout, stderr, shellCode := runGrouped(
			t,
			shellTimeout,
			root,
			[]string{"LC_ALL=" + locale},
			"bash",
			append([]string{script}, targets...)...,
		)
		var goOut, goErr bytes.Buffer
		goCode := skills.RunCheck(t.Context(), root, targets, &goOut, &goErr)
		require.Equal(t, shellCode, goCode)
		require.Equal(t, stdout, goOut.String())
		if traceback {
			// The old Python error dump is accidental, so Go omits it on purpose.
			// Findings, totals and status still match, and Go prints nothing to stderr.
			require.Contains(t, stderr, "Traceback (most recent call last):")
			require.Contains(t, stderr, "UnicodeDecodeError:")
			require.Empty(t, goErr.String(), "the old Python error dump is deliberately omitted")
		} else {
			// Bash 5 warns about each null byte it drops from a command substitution.
			// The warning is shell noise, and Go has no such step.
			var shellErr []string
			for line := range strings.Lines(stderr) {
				if !strings.Contains(line, "ignored null byte in input") {
					shellErr = append(shellErr, line)
				}
			}
			require.Equal(t, strings.Join(shellErr, ""), goErr.String())
		}
		require.Equal(t, before, snapshot(t, root), "AC11: repository bytes, index and refs")
	}
}

func TestFrozenParity(t *testing.T) {
	t.Parallel()
	for _, item := range corpus() {
		t.Run(item.label, func(t *testing.T) {
			t.Parallel()
			root := fixture(t)
			writeManifest(t, root, "skill", item.text)
			parity(t, root, []string{"missing/", "skill//", "skill"}, false)
		})
	}
}

func TestUnreadableParity(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	writeManifest(t, root, "valid", "---\nname: valid\ndescription: text\n---\n")
	writeManifest(t, root, "unreadable", "\xff")
	parity(t, root, []string{"valid", "unreadable"}, true)
}

func TestDiscoveryParity(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	for _, dir := range []string{"packages/agents/codex/.apm/skills/z", "packages/agents/claude/.apm/skills/a", ".claude/skills/a", ".claude/skills/local"} {
		writeManifest(t, root, dir, "body\n")
	}
	parity(t, root, nil, false)
	parity(t, root, []string{".claude/skills/local", "missing", "packages/agents/claude/.apm/skills/a/"}, false)
}

func TestHiddenDiscoveryParity(t *testing.T) {
	t.Parallel()
	for _, hidden := range []string{
		".claude/skills/.hidden",
		"packages/agents/codex/.apm/skills/.hidden",
		"packages/agents/.hidden/.apm/skills/local",
	} {
		t.Run(hidden, func(t *testing.T) {
			t.Parallel()
			root := fixture(t)
			writeManifest(t, root, hidden, "body\n")
			writeManifest(t, root, ".claude/skills/local", "body\n")
			parity(t, root, nil, false)
			// Explicit targets still include hidden directories.
			parity(t, root, []string{hidden}, false)
		})
	}
}

func TestExplicitTraversalParity(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	writeManifest(t, root, "skill", "---\nname: valid\ndescription: text\n---\n")
	writeManifest(t, root, "other/skill", "body\n")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "other/child"), 0o700))
	require.NoError(t, os.Symlink("other/child", filepath.Join(root, "link")))
	for _, target := range []string{"link/../skill", "missing/../other/skill"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			parity(t, root, []string{target}, false)
			parity(t, root, []string{root + "/" + target}, false)
		})
	}
}

func TestOfflineReadOnly(t *testing.T) {
	root := fixture(t)
	bin := t.TempDir()
	git, err := exec.LookPath("git")
	require.NoError(t, err)
	t.Setenv("SKILL_LIMIT_REAL_GIT", git)
	python, err := exec.CommandContext(t.Context(), "python3", "-c", "import sys; print(sys.executable)").Output()
	require.NoError(t, err)
	t.Setenv("SKILL_LIMIT_REAL_PYTHON", strings.TrimSpace(string(python)))
	trap := filepath.Join(t.TempDir(), "trap")
	t.Setenv("SKILL_LIMIT_TRAP", trap)
	source := filepath.Join(t.TempDir(), "trap.go")
	require.NoError(t, os.WriteFile(source, []byte(`package main
import ("os"; "os/exec"; "path/filepath"; "strings")
func main() {
	if os.Getenv("SKILL_LIMIT_IN_TRAP") != "" { os.Stderr.WriteString("trap re-entered; refusing to recurse\n"); os.Exit(97) }
	self, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	var path []string
	for _, d := range filepath.SplitList(os.Getenv("PATH")) { if d != self { path = append(path, d) } }
	env := append(os.Environ(), "SKILL_LIMIT_IN_TRAP=1", "PATH="+strings.Join(path, string(os.PathListSeparator)))
	if filepath.Base(os.Args[0]) == "git" && strings.Join(os.Args[1:], " ") == "rev-parse --show-toplevel" {
	c := exec.Command(os.Getenv("SKILL_LIMIT_REAL_GIT"), os.Args[1:]...)
	c.Env, c.Stdout, c.Stderr = env, os.Stdout, os.Stderr
	if c.Run() != nil { os.Exit(98) }; return
	}
	if filepath.Base(os.Args[0]) == "python3" && strings.Join(os.Args[1:], " ") == "- skill/SKILL.md" {
	c := exec.Command(os.Getenv("SKILL_LIMIT_REAL_PYTHON"), os.Args[1:]...)
	c.Env, c.Stdin, c.Stdout, c.Stderr = env, os.Stdin, os.Stdout, os.Stderr
	if c.Run() != nil { os.Exit(1) }; return
	}
	_ = os.WriteFile(os.Getenv("SKILL_LIMIT_TRAP"), []byte("unexpected command"), 0600)
	os.Exit(99)
}`), 0o600))
	executable := filepath.Join(bin, "git")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", executable, source)
	data, buildErr := build.CombinedOutput()
	require.NoError(t, buildErr, "%s", data)
	for _, tool := range []string{"curl", "wget", "ssh", "gh", "python3"} {
		require.NoError(t, os.Link(executable, filepath.Join(bin, tool)))
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, refusal, refusalCode := runGrouped(
		t,
		10*time.Second,
		root,
		[]string{"SKILL_LIMIT_IN_TRAP=1"},
		filepath.Join(bin, "python3"),
		"-",
		"skill/SKILL.md",
	)
	require.Equal(t, 97, refusalCode)
	require.Contains(t, refusal, "refusing to recurse")
	for _, text := range []string{"---\nname: valid\ndescription: text\n---\n", "body\n", "\xff"} {
		writeManifest(t, root, "skill", text)
		parity(t, root, []string{"skill"}, text == "\xff")
		require.NoFileExists(t, trap)
	}
}

func TestFrozenChecker(t *testing.T) {
	t.Parallel()
	base, err := os.ReadFile("../../tools/skill-limits.sh")
	require.NoError(t, err)
	frozen, err := os.ReadFile("../../tools/skill-limits-legacy.sh")
	require.NoError(t, err)
	require.Equal(t, base, frozen, "the frozen checker must be byte-identical to the base checker")
}
