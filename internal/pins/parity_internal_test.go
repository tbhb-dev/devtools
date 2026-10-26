// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package pins

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Shell parity uses controlled executables, never the network or deployed tools.
// The portable behavior suite also runs where a native Bash is unavailable.
func TestFrozenShellParity(t *testing.T) {
	t.Setenv("PIN_SCENARIO", "")
	if runtime.GOOS == "windows" {
		t.Skip("shell parity requires POSIX Bash; portable behavior runs natively")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Bash unavailable")
	}
	legacy := frozenShell(t)
	for _, scenario := range []string{"annotated", "lightweight", "absent", "failed-partial", "unsupported", "guard-markdown", "commit-lint", "gh-post", "minimal", "no-agentrepo", "harnesses", "unpinned", "no-comment", "no-lock", "downgrade", "tasks-failure", "vendir-failure", "lock-failure", "install-failure", "apm-failure", "check-failure"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("PIN_SCENARIO", scenario)
			compareShell(t, bash, legacy, scenario)
		})
	}
}

type parityResult struct {
	status         int
	stdout, stderr string
	files          map[string]string
}

func fixture(t *testing.T, dir, scenario string) {
	t.Helper()
	fixtureTools(t, dir, scenario)
	if scenario == "minimal" {
		return
	}
	for _, file := range []string{"apm.yml", ".github/renovate.json5", ".github/workflows/lint.yml"} {
		source := filepath.Join("../../tests/tasks/fixtures/pins", file)
		if _, err := os.Stat(source); err == nil {
			write(t, dir, file, read(t, filepath.Dir(source), filepath.Base(source)))
		}
	}
	write(t, dir, "vendir.yml", read(t, "../../.devtools", "vendir.yml"))
	write(
		t,
		dir,
		".pre-commit-config.yaml",
		"repo: https://github.com/tbhb-dev/devtools\n  rev: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  # frozen: v0.10.0\n",
	)
	write(
		t,
		dir,
		".github/workflows/extra.yaml",
		"uses: tbhb-dev/devtools/.github/workflows/go.yml@aaaaaaaa # v0.10.0\n",
	)
	write(t, dir, "tombi.toml", "# adapted from tbhb-dev/devtools v0.10.0.")
	write(t, dir, "README.md", "tbhb-dev/devtools v0.10.0\n")
	if scenario == "tombi-write-failure" {
		require.NoError(t, os.Mkdir(filepath.Join(dir, "tombi.toml.pins.tmp"), 0o700))
	}
	switch scenario {
	case "harnesses":
		write(
			t,
			dir,
			"apm.yml",
			"  - tbhb-dev/devtools#v0.10.0\n  - 'tbhb-dev/devtools/packages/agents/common#v0.10.0'\n  - tbhb-dev/devtools/packages/agents/agy#v0.10.0\n  - \"tbhb-dev/devtools/packages/agents/claude#v0.10.0\"\n  - tbhb-dev/devtools/packages/agents/codex#v0.10.0\n",
		)
	case "unsupported":
		write(t, dir, "apm.yml", "  - tbhb-dev/devtools/packages/unknown#v0.10.0\n")
	case "unpinned":
		write(t, dir, ".github/renovate.json5", "\"github>tbhb-dev/devtools//.github/custom.json5\"\n")
	case "no-comment":
		write(t, dir, ".github/renovate.json5", "\"github>tbhb-dev/devtools//.github/custom.json5#v0.10.0\"\n")
	}
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("%w", err)
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("%w", err)
		}
		files[strings.ReplaceAll(rel, "\\", "/")] = read(t, dir, rel)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func compareShell(t *testing.T, bash, legacy, scenario string) {
	t.Helper()
	results := shellResults(t, bash, legacy, scenario)
	require.Equal(t, results[0], results[1])
}

func shellResults(t *testing.T, bash, legacy, scenario string) []parityResult {
	t.Helper()

	results := make([]parityResult, 0, 2)
	for _, shell := range []bool{true, false} {
		dir, bin := t.TempDir(), t.TempDir()
		fixture(t, dir, scenario)
		trace := filepath.Join(dir, "trace")
		setupCommands(t, bin, trace, scenario)
		var out, stderr bytes.Buffer
		var status int
		if shell {
			cmd := exec.CommandContext(t.Context(), bash, legacy)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "usage_version=0.11.0")
			cmd.Stdout, cmd.Stderr = &out, &stderr
			if runErr := cmd.Run(); runErr != nil {
				var exit *exec.ExitError
				require.ErrorAs(t, runErr, &exit)
				status = exit.ExitCode()
			}
		} else {
			status = Run(context.Background(), dir, "0.11.0", &out, &stderr)
		}
		results = append(results, parityResult{status, out.String(), stderr.String(), snapshot(t, dir)})
	}
	return results
}

func TestTombiRewriteFailureIntendedDivergence(t *testing.T) {
	t.Setenv("PIN_SCENARIO", "")
	if runtime.GOOS == "windows" {
		t.Skip("shell comparison requires POSIX Bash")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Bash unavailable")
	}
	legacy := frozenShell(t)
	results := shellResults(t, bash, legacy, "tombi-write-failure")
	require.Zero(t, results[0].status)
	require.Contains(t, results[0].stdout, "delivered-checker")
	require.Equal(t, 1, results[1].status)
	require.Contains(t, results[1].stderr, "tombi.toml")
	require.NotContains(t, results[1].files["trace"], "vendir")
	require.Contains(t, results[1].files["apm.yml"], "#v0.11.0")
	require.Contains(t, results[1].files["mise.toml"], "v0.10.0")
	require.Equal(t, results[0].files["tombi.toml"], results[1].files["tombi.toml"])
}

func setupCommands(t *testing.T, bin, trace, scenario string) {
	t.Helper()

	stub := `#!/usr/bin/env bash
name=${0##*/}
printf '%s %s\n' "$name" "$*" >> "$PIN_TRACE"
case "$name $*" in
	git*)
		case "$PIN_SCENARIO" in
			absent) exit 0 ;;
			failed-partial) printf 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\tref\n'; echo resolution-failed >&2; exit 23 ;;
			lightweight) case "$*" in *'{}') exit 0 ;; esac ;;
		esac
		printf 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\tref\n' ;;
	'vendir sync')
		for key in GIT_DIR GIT_COMMON_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY; do
			if [ -n "${!key:-}" ]; then echo "leaked $key" >&2; exit 31; fi
		done
		[ "$PIN_SCENARIO" != vendir-failure ] || exit 19 ;;
	'mise lock --platform linux-amd64,windows-amd64') [ "$PIN_SCENARIO" != lock-failure ] || exit 19 ;;
	'mise install') [ "$PIN_SCENARIO" != install-failure ] || exit 19 ;;
	'mise run devtools:apm-sync') [ "$PIN_SCENARIO" != apm-failure ] || exit 19 ;;
	'mise tasks ls --all')
		[ "$PIN_SCENARIO" != downgrade ] || exit 0
		echo 'devtools:check-pins delivered'
		[ "$PIN_SCENARIO" != tasks-failure ] || exit 19 ;;
	'mise run devtools:check-pins') echo delivered-checker; [ "$PIN_SCENARIO" != check-failure ] || exit 19 ;;
esac
exit 0
`
	for _, name := range []string{"git", "mise", "vendir"} {
		write(t, bin, name, stub)
		info, err := os.Stat(filepath.Join(bin, name))
		require.NoError(t, err)
		require.NoError(t, os.Chmod(filepath.Join(bin, name), info.Mode()|0o100))
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PIN_TRACE", trace)
	t.Setenv("PIN_SCENARIO", scenario)
	t.Setenv("DEVTOOLS_LOCK_PLATFORMS", "linux-amd64,windows-amd64")
	if scenario == "no-lock" {
		t.Setenv("DEVTOOLS_LOCK_PLATFORMS", "")
	}
	for _, key := range []string{"GIT_DIR", "GIT_COMMON_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY"} {
		t.Setenv(key, "inherited")
	}
}

func fixtureTools(t *testing.T, dir, scenario string) {
	t.Helper()

	for _, tool := range []string{"guard-markdown", "commit-lint", "gh-post", "agentrepo"} {
		if scenario != tool && (scenario != "no-agentrepo" || tool != "agentrepo") {
			f, err := os.OpenFile(filepath.Join(dir, "mise.toml"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			_, err = fmt.Fprintf(f, "\"go:github.com/tbhb-dev/devtools/cmd/%s\" = \"v0.10.0\"\n", tool)
			if err != nil {
				t.Fatal(err)
			}
			if err = f.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func frozenShell(t *testing.T) string {
	t.Helper()
	body := read(t, "../../.devtools/tasks", "pins.toml")
	_, section, found := strings.Cut(body, `["devtools:update-pins-legacy"]`)
	require.True(t, found)
	_, script, found := strings.Cut(section, "run = "+strings.Repeat("'", 3)+"\n")
	require.True(t, found)
	script, _, found = strings.Cut(script, strings.Repeat("'", 3)+"\n")
	require.True(t, found)
	dir := t.TempDir()
	write(t, dir, "legacy.sh", script)
	return filepath.Join(dir, "legacy.sh")
}
