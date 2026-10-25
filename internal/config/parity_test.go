// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package config_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type result struct {
	out, stderr string
	code        int
}

func runCommand(t *testing.T, dir string, env []string, args ...string) result {
	t.Helper()
	//nolint:gosec // Commands are test-controlled; isolated fixtures supply their environment.
	cmd := exec.CommandContext(t.Context(), args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if cmd.ProcessState == nil {
			t.Fatal(err)
		}
		code = cmd.ProcessState.ExitCode()
	}
	return result{out: out.String(), stderr: stderr.String(), code: code}
}

func isolatedEnv(f fixture) []string {
	var env []string
	for _, v := range os.Environ() {
		key, _, _ := strings.Cut(v, "=")
		if strings.HasPrefix(key, "GIT_") || strings.HasPrefix(key, "DEVTOOLS_") || key == "HOME" ||
			key == "XDG_CONFIG_HOME" ||
			key == "BASH_ENV" ||
			key == "ENV" {
			continue
		}
		env = append(env, v)
	}
	env = append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	for key, v := range f.env {
		env = append(env, key+"="+v)
	}
	return env
}

func gitFixture(t *testing.T, f fixture, args ...string) {
	t.Helper()
	r := runCommand(t, f.root, isolatedEnv(f), append([]string{"git"}, args...)...)
	if r.code != 0 {
		t.Fatalf("git %v: %+v", args, r)
	}
}

func assertParity(t *testing.T, f fixture, binary, legacy, kind, key, fallback string) {
	t.Helper()
	oldFunc := "dt_conf"
	if kind == "repository" {
		oldFunc = "dt_repo_conf"
	}
	env := isolatedEnv(f)
	path, ok := f.env["PATH"]
	if !ok {
		path = os.Getenv("PATH")
	}
	old := runCommand(t, f.root, env, "bash", legacy, oldFunc, key, fallback)
	got := runCommand(t, f.root, env, binary, "config", kind, "--", key, fallback)
	if got != old {
		t.Fatalf("old: %+v\nnew: %+v", old, got)
	}
	for _, harness := range []string{"claude", "codex"} {
		lib, err := filepath.Abs("../../packages/agents/" + harness + "/.apm/skills/commit/scripts/lib.sh")
		if err != nil {
			t.Fatal(err)
		}
		for _, bin := range []string{filepath.Dir(binary), filepath.Join(filepath.Dir(binary), "no-config")} {
			got = runCommand(t, f.root, append(env, "PATH="+bin+string(os.PathListSeparator)+path),
				"bash", "-c", `. "$1"; "$2" "$3" "$4"`, "parity", lib, oldFunc, key, fallback)
			if got != old {
				t.Fatalf("%s with %s: old: %+v\nwrapper: %+v", harness, bin, old, got)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(f.root, "PWNED")); !os.IsNotExist(err) {
		t.Fatal("configuration was evaluated")
	}
}

func TestConfigParity(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("legacy shell oracle uses POSIX paths; native tests run on Windows")
	}
	for _, tool := range []string{"bash", "awk", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("legacy parity requires %s", tool)
		}
	}
	binary := filepath.Join(t.TempDir(), "agentrepo")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../../cmd/agentrepo")
	if data, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, data)
	}
	oldBin := filepath.Join(filepath.Dir(binary), "no-config")
	if err := os.Mkdir(oldBin, 0o700); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(oldBin, "agentrepo")
	writeFile(t, stub, `#!/bin/sh
if [ "$1" = --version ]; then echo 'agentrepo version v0.16.0'; exit 0; fi
printf 'unknown command config\n' >&2
exit 5
`)
	//nolint:gosec // This fixture models an installed agentrepo without config.
	if err := os.Chmod(stub, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy, err := filepath.Abs("testdata/legacy-config.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range configCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, tc)
			gitFixture(t, f, "init", "--quiet", "--initial-branch=main")
			assertParity(t, f, binary, legacy, tc.kind, tc.key, tc.fallback)
		})
	}
	for _, support := range []string{"true", "false"} {
		t.Run("config support is cached: "+support, func(t *testing.T) {
			t.Parallel()
			parityProbe(t, binary, support)
		})
	}
	t.Run("linked worktree uses main name and worktree behavior", func(t *testing.T) {
		t.Parallel()
		parityWorktree(t, binary, legacy)
	})
	t.Run("failed Git output cannot become configuration", func(t *testing.T) {
		t.Parallel()
		parityFailedGit(t, binary, legacy)
	})
	for _, kind := range []string{"directory", "symlink", "dangling symlink"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, configCase{})
			gitFixture(t, f, "init", "--quiet", "--initial-branch=main")
			configFileKind(t, f, kind)
			assertParity(t, f, binary, legacy, "repository", "hook_marker", "fallback")
		})
	}
}

func configFileKind(t *testing.T, f fixture, kind string) {
	t.Helper()
	path := filepath.Join(f.root, ".agents", "agent-skills.conf")
	if kind == "directory" {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		return
	}
	target := filepath.Join(t.TempDir(), "target")
	if kind == "symlink" {
		writeFile(t, target, "hook_marker = value\n")
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func TestRepositoryFileKinds(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"directory", "symlink", "dangling symlink"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, configCase{})
			configFileKind(t, f, kind)
			got, err := f.lookup.Repository("hook_marker", "fallback")
			if kind == "dangling symlink" {
				if err != nil || got != "fallback" {
					t.Fatalf("%q, %v", got, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "must be a regular file") || got != "" {
				t.Fatalf("%q, %v", got, err)
			}
		})
	}
}

func parityWorktree(t *testing.T, binary, legacy string) {
	t.Helper()
	f := newFixture(t, configCase{machine: "review_cmd = main-name\n"})
	gitFixture(t, f, "init", "--quiet", "--initial-branch=main")
	gitFixture(
		t,
		f,
		"-c",
		"user.name=Tester",
		"-c",
		"user.email=tester@example.com",
		"-c",
		"core.hooksPath=/dev/null",
		"commit",
		"--quiet",
		"--allow-empty",
		"-m",
		"fixture",
	)
	worktree := filepath.Join(t.TempDir(), "linked")
	gitFixture(t, f, "worktree", "add", "--quiet", "-b", "linked", worktree)
	f.root = worktree
	if err := os.MkdirAll(filepath.Join(worktree, ".agents"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(worktree, ".agents", "agent-skills.conf"), "hook_marker = linked-value\n")
	assertParity(t, f, binary, legacy, "machine", "review_cmd", "fallback")
	assertParity(t, f, binary, legacy, "repository", "hook_marker", "fallback")
	got := runCommand(t, f.root, isolatedEnv(f), binary, "config", "machine", "review_cmd")
	if got.out != "main-name\n" || got.code != 0 {
		t.Fatal(got)
	}
}

func parityFailedGit(t *testing.T, binary, legacy string) {
	t.Helper()
	f := newFixture(t, configCase{})
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bin, "git")
	writeFile(t, path, "#!/bin/sh\nprintf '%s/.git\\n' \"$PWD\"\nexit 9\n")
	//nolint:gosec // This fixture must be executable as fake git.
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	f.env["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
	for _, kind := range []string{"machine", "repository"} {
		assertParity(t, f, binary, legacy, kind, "review_cmd", "fallback")
		got := runCommand(t, f.root, isolatedEnv(f), binary, "config", kind, "review_cmd", "fallback")
		if got.code != 9 || got.out != "" {
			t.Fatal(got)
		}
	}
	f.env["DEVTOOLS_REVIEW_CMD"] = ""
	assertParity(t, f, binary, legacy, "machine", "review_cmd", "fallback")
}

func parityProbe(t *testing.T, binary, support string) {
	t.Helper()
	f := newFixture(t, configCase{machine: "review_cmd = machine\n", repository: "hook_marker = repository\n"})
	gitFixture(t, f, "init", "--quiet", "--initial-branch=main")
	bin := t.TempDir()
	stub := filepath.Join(bin, "agentrepo")
	writeFile(t, stub, `#!/bin/sh
printf '%s\n' "$*" >>"$CONFIG_LOG"
if [ "$1" = --version ]; then exec "$CONFIG_BINARY" "$@"; fi
[ "$CONFIG_SUPPORT" = true ] || exit 5
exec "$CONFIG_BINARY" "$@"
`)
	//nolint:gosec // Executable shim records probes and forwards to the real binary.
	if err := os.Chmod(stub, 0o700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(bin, "calls")
	env := append(isolatedEnv(f), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CONFIG_SUPPORT="+support, "CONFIG_BINARY="+binary, "CONFIG_LOG="+log)
	lib, err := filepath.Abs("../../packages/agents/codex/.apm/skills/commit/scripts/lib.sh")
	if err != nil {
		t.Fatal(err)
	}
	got := runCommand(t, f.root, env, "bash", "-c", `. "$1"
machine=$(dt_conf review_cmd fallback)
repository=$(dt_repo_conf hook_marker fallback)
printf '%s\n' "$machine" "$repository"
dt_conf review_cmd fallback
dt_repo_conf hook_marker fallback
`, "probe", lib)
	if got != (result{out: "machine\nrepository\nmachine\nrepository\n"}) {
		t.Fatal(got)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := "--version\nconfig --help\n"
	if support == "true" {
		want += strings.Repeat(
			"config machine -- review_cmd fallback\nconfig repository -- hook_marker fallback\n", 2,
		)
	}
	if string(calls) != want {
		t.Fatalf("calls: %q; want %q", calls, want)
	}
}
