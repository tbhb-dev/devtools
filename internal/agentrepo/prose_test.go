// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

type (
	proseFixture struct{ root, script string }
	proseResult  struct {
		Code     int
		Out, Err string
	}
)

func newProseFixture(t *testing.T, label string) proseFixture {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	require.True(t, ok)
	project := filepath.Join(filepath.Dir(source), "../..")
	root := filepath.Join(project, fmt.Sprintf(".prose-test-%x", sha256.Sum256([]byte(t.Name()+label))))
	require.NoError(t, os.Mkdir(root, 0o700))
	root, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(root)) })
	f := proseFixture{
		root,
		filepath.Join(project, "packages/agents/source/skills/fix-prose/shared/scripts/check-suppressions.sh"),
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_") {
			t.Setenv(key, "")
			require.NoError(t, os.Unsetenv(key))
		}
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	f.git(t, "init", "-q")
	f.write(t, "DRAFT.md", "committed draft\n")
	f.write(t, ".gitignore", "ignored.md\n")
	f.git(t, "add", "DRAFT.md", ".gitignore")
	f.git(t, "commit", "-qm", "fixture")
	return f
}

func (f proseFixture) git(t *testing.T, args ...string) {
	t.Helper()
	c := exec.CommandContext(t.Context(), "git")
	c.Args = append(c.Args, "-c", "core.hooksPath="+os.DevNull, "-c", "commit.gpgsign=false")
	c.Args = append(c.Args, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com")
	c.Args = append(c.Args, args...)
	c.Dir = f.root
	out, err := c.CombinedOutput()
	require.NoError(t, err, "%s", out)
}

func (f proseFixture) write(t *testing.T, path, content string) {
	t.Helper()
	path = filepath.Join(f.root, path)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func (f proseFixture) run(t *testing.T, shell bool, args ...string) proseResult {
	t.Helper()
	return f.runIn(t, f.root, shell, args...)
}

func (f proseFixture) runIn(t *testing.T, dir string, shell bool, args ...string) proseResult {
	t.Helper()
	var out, stderr bytes.Buffer
	code := 0
	if shell {
		c := exec.CommandContext(t.Context(), "bash")
		c.Args = append(c.Args, f.script)
		c.Args = append(c.Args, args...)
		c.Dir, c.Stdout, c.Stderr = dir, &out, &stderr
		if err := c.Run(); err != nil {
			exit, ok := errors.AsType[*exec.ExitError](err)
			require.True(t, ok, "%v", err)
			code = exit.ExitCode()
		}
	} else {
		t.Chdir(dir)
		code = agentrepo.Run(append([]string{"prose", "check-suppressions"}, args...), nil, &out, &stderr)
	}
	return proseResult{code, out.String(), stderr.String()}
}

func (f proseFixture) state(target string) string {
	return filepath.Join(f.root, ".fix-prose", fmt.Sprintf("run.%x", sha256.Sum256([]byte(target))))
}

func (f proseFixture) shellBaseline(t *testing.T, target string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		require.Equal(t, 0, f.run(t, true, "--baseline", target).Code)
		return
	}
	// Recorded shell layout for an otherwise clean checkout and an existing target.
	data, err := os.ReadFile(filepath.Join(f.root, target))
	require.NoError(t, err)
	f.write(t, ".fix-prose/.gitignore", "*\n")
	for name, value := range map[string]string{"target": target + "\n", "original": string(data), "digest": "", "directives": ""} {
		f.write(t, filepath.Join(".fix-prose", filepath.Base(f.state(target)), name), value)
	}
}

func (f proseFixture) evidence(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	root := os.DirFS(f.root)
	for _, dir := range []string{".fix-prose", ".git/fix-prose", "wrong"} {
		err := fs.WalkDir(root, dir, func(path string, entry fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			require.NoError(t, err)
			if !entry.IsDir() {
				data, readErr := fs.ReadFile(root, path)
				require.NoError(t, readErr)
				files[path] = string(data)
			}
			return nil
		})
		require.NoError(t, err)
	}
	return files
}

func proseMetadataMode(t *testing.T, f proseFixture, readonly bool) {
	t.Helper()
	paths := map[string]fs.FileMode{}
	require.NoError(t, fs.WalkDir(os.DirFS(f.root), ".git", func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		mode := fs.FileMode(0o600)
		if d.IsDir() {
			mode = 0o700
		}
		if readonly {
			mode &^= 0o200
		}
		paths[path] = mode
		return nil
	}))
	for path, mode := range paths {
		require.NoError(t, os.Chmod(filepath.Join(f.root, path), mode))
	}
}

func proseNormalize(root, value string) string {
	value = strings.ReplaceAll(value, root, "<root>")
	return regexp.MustCompile(`(?m)^(---|\+\+\+) ([^\n]*)\t[^\n]*`).ReplaceAllString(value, "$1 $2\t<time>")
}

func proseCompare(t *testing.T, a, b proseFixture, got, want proseResult) {
	t.Helper()
	require.Equal(t, want.Code, got.Code)
	require.Equal(t, proseNormalize(b.root, want.Out), proseNormalize(a.root, got.Out))
	require.Equal(t, proseNormalize(b.root, want.Err), proseNormalize(a.root, got.Err))
	require.Equal(t, b.evidence(t), a.evidence(t))
}

func proseShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("live shell oracle requires Unix; native assertions use recorded shell fixtures on Windows")
	}
}

func TestProseState(t *testing.T) {
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Run("Baseline", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
		for _, kind := range []string{"dirty", "staged", "untracked", "ignored", "non-ASCII", "read-only"} {
			t.Run(kind, func(t *testing.T) {
				t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
				proseBaselineCase(t, kind)
			})
		}
	})
	for _, name := range []string{"DuplicateBaseline", "DistinctTargets", "TargetMismatch", "InvalidShellCreatedState", "CleanupIsolation", "NestedDirectory"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			proseStateCase(t, name)
		})
	}
}

func proseCleanup(t *testing.T, a, b proseFixture) {
	t.Helper()
	for _, legacy := range []bool{false, true} {
		if legacy {
			require.Equal(t, 0, a.run(t, false, "--baseline", "OTHER.md").Code)
			require.Equal(t, 0, b.run(t, runtime.GOOS != "windows", "--baseline", "OTHER.md").Code)
		}
		states := []string{a.state("OTHER.md"), b.state("OTHER.md")}
		if legacy {
			for i, f := range []proseFixture{a, b} {
				states[i] = filepath.Join(f.root, ".git/fix-prose", filepath.Base(states[i]))
				require.NoError(t, os.MkdirAll(filepath.Dir(states[i]), 0o700))
				require.NoError(t, os.Rename(f.state("OTHER.md"), states[i]))
			}
		}
		supplied := states[0]
		if !legacy {
			supplied = ".fix-prose/" + filepath.Base(supplied)
		}
		got := a.run(t, false, "--cleanup", supplied)
		require.Equal(t, proseResult{}, got)
		require.NoDirExists(t, states[0])
		require.DirExists(t, a.state("DRAFT.md"))
		t.Run(fmt.Sprint("ShellOracleLegacy", legacy), func(t *testing.T) {
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			proseShell(t)
			proseCompare(t, a, b, got, b.run(t, true, "--cleanup", states[1]))
		})
	}
}

func TestProseDiff(t *testing.T) {
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, name := range []string{"ChangedTarget", "UnchangedTarget", "MissingOriginal"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			for _, legacy := range []bool{false, true} {
				t.Run(fmt.Sprint("Legacy", legacy), func(t *testing.T) {
					proseDiffCase(t, name, legacy)
				})
			}
		})
	}
}

func TestProseStateWorkflowStateOnlyEdits(t *testing.T) {
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	f := newProseFixture(t, "a")
	require.Equal(t, 0, f.run(t, false, "--baseline", "DRAFT.md").Code)
	before := f.evidence(t)
	f.write(t, filepath.Join(".fix-prose", filepath.Base(f.state("DRAFT.md")), "notes"), "run note\n")
	require.Empty(t, before[".fix-prose/"+filepath.Base(f.state("DRAFT.md"))+"/digest"])
	t.Run("ShellOracle", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
		proseShell(t)
		require.Equal(t, proseResult{}, f.run(t, true, "--verify", f.state("DRAFT.md"), "DRAFT.md"))
	})
	require.NoError(t, os.Rename(f.state("DRAFT.md"), filepath.Join(f.root, ".fix-prose/run.retained")))
	require.Equal(t, 0, f.run(t, false, "--baseline", "DRAFT.md").Code)
	key := ".fix-prose/" + filepath.Base(f.state("DRAFT.md")) + "/digest"
	require.Equal(t, before[key], f.evidence(t)[key])
}

func TestProseStateGoCreatedShellOperations(t *testing.T) {
	for _, name := range []string{"VerifyUnchanged", "DiffChanged", "CleanupIsolation"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			proseShell(t)
			a, b := newProseFixture(t, "a"), newProseFixture(t, "b")
			for i, f := range []proseFixture{a, b} {
				f.write(t, "DRAFT.md", "first draft\n<!-- vale off -->\n")
				f.write(t, "OTHER.md", "other\n")
				f.write(t, ".cspell-words.txt", "example\n")
				f.write(t, ".vale/config/vocabularies/project/accept.txt", "example\n")
				f.write(t, "stray.txt", "dirty\n")
				require.Equal(t, 0, f.run(t, i == 1, "--baseline", "DRAFT.md").Code)
				require.Equal(t, 0, f.run(t, i == 1, "--baseline", "OTHER.md").Code)
			}
			require.Equal(t, b.evidence(t), a.evidence(t))
			if name == "DiffChanged" {
				a.write(t, "DRAFT.md", "second draft\n")
				b.write(t, "DRAFT.md", "second draft\n")
			}
			mode := map[string]string{"VerifyUnchanged": "--verify", "DiffChanged": "--diff", "CleanupIsolation": "--cleanup"}[name]
			got, want := a.run(t, true, mode, a.state("DRAFT.md")), b.run(t, true, mode, b.state("DRAFT.md"))
			proseCompare(t, a, b, got, want)
			require.Equal(t, 0, got.Code, got)
			if mode == "--verify" {
				require.Equal(t, proseResult{}, got)
			}
			if mode == "--cleanup" {
				require.NoDirExists(t, a.state("DRAFT.md"))
				require.DirExists(t, a.state("OTHER.md"))
			}
		})
	}
}

func proseBaselineCase(t *testing.T, kind string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	a, b := newProseFixture(t, "a"), newProseFixture(t, "b")
	target := "DRAFT.md"
	switch kind {
	case "untracked":
		target = "new.md"
	case "ignored":
		target = "ignored.md"
	case "non-ASCII":
		target = "café.md"
	}
	for _, f := range []proseFixture{a, b} {
		f.write(t, target, "first draft\n")
		if kind == "staged" {
			f.git(t, "add", target)
		}
		if kind == "read-only" && runtime.GOOS != "windows" {
			proseMetadataMode(t, f, true)
			t.Cleanup(func() { proseMetadataMode(t, f, false) })
		}
	}
	got := a.run(t, false, "--baseline", "./"+target)
	require.Equal(t, proseResult{0, "state: " + a.state(target) + "\n", ""}, got)
	key := ".fix-prose/" + filepath.Base(a.state(target)) + "/"
	require.Equal(
		t,
		map[string]string{
			".fix-prose/.gitignore": "*\n",
			key + "target":          target + "\n",
			key + "original":        "first draft\n",
			key + "digest":          "",
			key + "directives":      "",
		},
		a.evidence(t),
	)
	t.Run("ShellOracle", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
		proseShell(t)
		proseCompare(t, a, b, got, b.run(t, true, "--baseline", "./"+target))
	})
}

func proseStateCase(t *testing.T, name string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	a, b := newProseFixture(t, "a"), newProseFixture(t, "b")
	for i, f := range []proseFixture{a, b} {
		if i == 1 || name == "InvalidShellCreatedState" {
			f.shellBaseline(t, "DRAFT.md")
		} else {
			require.Equal(t, 0, f.run(t, false, "--baseline", "DRAFT.md").Code)
		}
		f.write(t, "OTHER.md", "other\n")
	}
	switch name {
	case "DuplicateBaseline":
		proseDuplicate(t, a, b)
	case "DistinctTargets", "CleanupIsolation":
		proseDistinct(t, a, b, name)
	case "TargetMismatch":
		proseMismatch(t, a, b)
	case "InvalidShellCreatedState":
		proseInvalid(t, a, b)
	case "NestedDirectory":
		proseNested(t, a, b)
	}
}

func proseDiffCase(t *testing.T, name string, legacy bool) {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	a, b := newProseFixture(t, "a"), newProseFixture(t, "b")
	for _, f := range []proseFixture{a, b} {
		f.write(t, "DRAFT.md", "first draft\n")
		f.shellBaseline(t, "DRAFT.md")
	}
	states := []string{a.state("DRAFT.md"), b.state("DRAFT.md")}
	for i, f := range []proseFixture{a, b} {
		if legacy {
			states[i] = filepath.Join(f.root, ".git/fix-prose", filepath.Base(states[i]))
			require.NoError(t, os.MkdirAll(filepath.Dir(states[i]), 0o700))
			require.NoError(t, os.Rename(f.state("DRAFT.md"), states[i]))
		}
		switch name {
		case "ChangedTarget":
			f.write(t, "DRAFT.md", "second draft\n")
		case "MissingOriginal":
			require.NoError(t, os.Remove(filepath.Join(states[i], "original")))
		}
	}
	got := a.run(t, false, "--diff", states[0], "DRAFT.md")
	want := proseResult{}
	switch name {
	case "ChangedTarget":
		want.Out = "--- " + filepath.Join(
			states[0],
			"original",
		) + "\t<time>\n+++ DRAFT.md\t<time>\n@@ -1 +1 @@\n-first draft\n+second draft\n"
	case "MissingOriginal":
		want = proseResult{2, "", "check-suppressions: baseline kept no copy of DRAFT.md\n"}
	}
	require.Equal(t, proseNormalize(a.root, want.Out), proseNormalize(a.root, got.Out))
	require.Equal(t, want.Code, got.Code)
	require.Equal(t, want.Err, got.Err)
	t.Run("ShellOracle", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
		proseShell(t)
		proseCompare(t, a, b, got, b.run(t, true, "--diff", states[1], "DRAFT.md"))
	})
}

func proseDuplicate(t *testing.T, a, b proseFixture) {
	t.Helper()

	got := a.run(t, false, "--baseline", "DRAFT.md")
	const guidance = "check-suppressions: an active workflow already owns DRAFT.md; if that run is abandoned, clear it with: check-suppressions.sh --cleanup %s\n"
	require.Equal(t, proseResult{2, "", fmt.Sprintf(guidance, a.state("DRAFT.md"))}, got)

	t.Run("ShellOracle", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
		proseShell(t)
		proseCompare(t, a, b, got, b.run(t, true, "--baseline", "DRAFT.md"))
	})
}

func proseDistinct(t *testing.T, a, b proseFixture, name string) {
	t.Helper()

	got := a.run(t, false, "--baseline", "OTHER.md")
	require.Equal(t, 0, got.Code, got)
	require.NotEqual(t, a.state("OTHER.md"), a.state("DRAFT.md"))
	t.Run("ShellOracle", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
		proseShell(t)
		proseCompare(t, a, b, got, b.run(t, true, "--baseline", "OTHER.md"))
	})
	if name == "CleanupIsolation" {
		proseCleanup(t, a, b)
	}
}

func proseMismatch(t *testing.T, a, b proseFixture) {
	t.Helper()

	got := a.run(t, false, "--diff", a.state("DRAFT.md"), "OTHER.md")
	require.Equal(t, proseResult{2, "", "check-suppressions: state belongs to DRAFT.md, not OTHER.md\n"}, got)
	t.Run("ShellOracle", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
		proseShell(t)
		for _, mode := range []string{"--diff", "--verify"} {
			proseCompare(t, a, b, got, b.run(t, true, mode, b.state("DRAFT.md"), "OTHER.md"))
		}
	})
}

func proseInvalid(t *testing.T, a, b proseFixture) {
	t.Helper()

	for _, f := range []proseFixture{a, b} {
		require.NoError(t, os.CopyFS(filepath.Join(f.root, "wrong/run.copy"), os.DirFS(f.state("DRAFT.md"))))
	}
	for _, path := range []string{"bad", "wrong/run.copy", ".fix-prose/run.missing"} {
		got := a.run(t, false, "--diff", filepath.Join(a.root, path), "DRAFT.md")
		label := "invalid workflow state"
		if strings.HasPrefix(path, ".fix-prose/") {
			label = "workflow state is missing its target"
		}
		require.Equal(
			t,
			proseResult{2, "", "check-suppressions: " + label + ": " + filepath.Join(a.root, path) + "\n"},
			got,
		)
		t.Run(path, func(t *testing.T) {
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			proseShell(t)
			proseCompare(t, a, b, got, b.run(t, true, "--diff", filepath.Join(b.root, path), "DRAFT.md"))
		})
	}
	proseMismatch(t, a, b)
}

// proseNested runs a relative target from a subdirectory, which the shell resolves at the checkout root.
func proseNested(t *testing.T, a, b proseFixture) {
	t.Helper()
	for _, f := range []proseFixture{a, b} {
		require.NoError(t, os.MkdirAll(filepath.Join(f.root, "sub"), 0o700))
	}
	shell := runtime.GOOS != "windows"
	want := b.runIn(t, filepath.Join(b.root, "sub"), shell, "--diff", b.state("DRAFT.md"), "DRAFT.md")
	got := a.runIn(t, filepath.Join(a.root, "sub"), false, "--diff", a.state("DRAFT.md"), "DRAFT.md")
	require.Equal(t, []int{0, 0}, []int{want.Code, got.Code}, want.Err+got.Err)
}
