// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package pins

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateValidationAndResolution(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"unsupported", "guard-markdown", "commit-lint", "gh-post", "absent", "failed-partial", "annotated", "lightweight"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			validationScenario(t, scenario)
		})
	}
}

func validationScenario(t *testing.T, scenario string) {
	t.Helper()
	dir := t.TempDir()
	var toolsBuilder strings.Builder
	toolsBuilder.WriteString("[tools]\n")
	for _, tool := range []string{"guard-markdown", "commit-lint", "gh-post"} {
		if tool != scenario {
			toolsBuilder.WriteString(`"go:github.com/tbhb-dev/devtools/cmd/` + tool + `" = "v0.10.0"` + "\n")
		}
	}
	tools := toolsBuilder.String()
	apm := "  - tbhb-dev/devtools#v0.10.0\n"
	if scenario == "unsupported" {
		apm = "  - tbhb-dev/devtools/packages/unknown#v0.10.0\n"
	}
	write(t, dir, "mise.toml", tools)
	write(t, dir, "apm.yml", apm)
	var out, stderr bytes.Buffer
	var calls []string
	u := updater{dir: dir, out: &out, stderr: &stderr}
	u.run = resolutionRunner(scenario, &calls)

	status := u.update(context.Background(), "0.11.0")
	success := scenario == "annotated" || scenario == "lightweight"
	if success {
		require.Zero(t, status, stderr.String())
		if got := read(t, dir, "apm.yml"); got != strings.ReplaceAll(apm, "v0.10.0", "v0.11.0") {
			t.Fatal(got)
		}
		return
	}
	want := 1
	if scenario == "failed-partial" {
		want = 23
	}
	require.Equal(t, want, status)
	require.Equal(t, tools, read(t, dir, "mise.toml"))
	require.Equal(t, apm, read(t, dir, "apm.yml"))
	for _, call := range calls {
		require.True(t, strings.HasPrefix(call, "git "), call)
	}
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestUpdateRefreshOrderAndFailures(t *testing.T) {
	for _, lock := range []string{"", "linux-amd64,windows-amd64"} {
		t.Run(lock, func(t *testing.T) {
			t.Setenv("DEVTOOLS_LOCK_PLATFORMS", lock)
			want := []string{"vendir sync"}
			if lock != "" {
				want = append(want, "mise lock --platform "+lock)
			}
			want = append(
				want,
				"mise install",
				"mise run devtools:apm-sync",
				"mise tasks ls --all",
				"mise run devtools:check-pins",
			)
			for fail := -1; fail < len(want); fail++ {
				checkRefresh(t, want, fail)
			}
		})
	}
}

func TestUpdateRewriteSites(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	inputs := map[string]string{
		"apm.yml":                  "  - tbhb-dev/devtools#v0.10.0\n  - 'tbhb-dev/devtools/packages/agents/common#v0.10.0'\n  - tbhb-dev/devtools/packages/agents/agy#v0.10.0\n  - \"tbhb-dev/devtools/packages/agents/claude#v0.10.0\"\n  - tbhb-dev/devtools/packages/agents/codex#v0.10.0\n",
		"mise.toml":                "[tools]\n",
		"tombi.toml":               "# adapted from tbhb-dev/devtools v0.10.0.",
		".github/workflows/a.yml":  "uses: tbhb-dev/devtools/.github/workflows/a.yml@aaaaaaaa # v0.10.0\n",
		".github/workflows/b.yaml": "uses: tbhb-dev/devtools/.github/workflows/b.yml@aaaaaaaa # v0.10.0\n",
		".github/renovate.json5":   "\"github>tbhb-dev/devtools//.github/custom.json5#aaaaaaaa\", // v0.10.0\n\"github>tbhb-dev/devtools//.github/no-comment.json5#v0.10.0\"\n\"github>tbhb-dev/devtools//.github/unpinned.json5\"\n",
		"vendir.yml":               "url: https://github.com/tbhb-dev/devtools\n  ref: v0.10.0\nurl: https://example.com/other\n  ref: keep\nurl: https://github.com/tbhb-dev/devtools\n  ref: v0.10.0\n",
		".pre-commit-config.yaml":  "repo: https://github.com/tbhb-dev/devtools\n  rev: aaaaaaaa  # frozen: v0.10.0\n",
		"README.md":                "tbhb-dev/devtools v0.10.0\n",
	}
	for _, tool := range []string{"guard-markdown", "commit-lint", "gh-post", "agentrepo"} {
		inputs["mise.toml"] += `"go:github.com/tbhb-dev/devtools/cmd/` + tool + `" = "v0.10.0"` + "\n"
	}
	for name, body := range inputs {
		write(t, dir, name, body)
	}
	u := updater{dir: dir}
	if err := u.rewrite("v0.11.0", "bbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	for name, body := range inputs {
		want := strings.ReplaceAll(strings.ReplaceAll(body, "v0.10.0", "v0.11.0"), "aaaaaaaa", "bbbbbbbb")
		switch name {
		case "README.md":
			want = body
		case "tombi.toml":
			want += "\n"
		case ".github/renovate.json5":
			want = strings.ReplaceAll(want, "no-comment.json5#v0.11.0", "no-comment.json5#bbbbbbbb")
		}
		if got := read(t, dir, name); got != want {
			t.Errorf("%s:\ngot %q\nwant %q", name, got, want)
		}
	}
}

func resolutionRunner(scenario string, calls *[]string) runner {
	return func(_ context.Context, name string, args []string, _ bool, stdout, _ io.Writer) int {
		call := name + " " + strings.Join(args, " ")
		*calls = append(*calls, call)
		if name == "git" {
			if scenario == "failed-partial" {
				fmt.Fprintln(stdout, "bbbb\trefs/tags/v0.11.0^{}")
				return 23
			}
			if scenario == "absent" || scenario == "lightweight" && strings.HasSuffix(call, "^{}") {
				return 0
			}
			fmt.Fprintln(stdout, "bbbb\trefs/tags/v0.11.0")
		}
		return 0
	}
}

func checkRefresh(t *testing.T, want []string, fail int) {
	t.Helper()

	var calls []string
	var out, stderr bytes.Buffer
	u := updater{out: &out, stderr: &stderr}
	u.run = refreshRunner(t, &calls, fail)
	status := u.refresh(context.Background(), "v0.11.0")
	end := len(want)
	expected := 0
	if fail >= 0 {
		end = fail + 1
		expected = 19
	}
	if fail == len(want)-2 {
		expected = 0
	} // An unavailable checker is a downgrade warning.
	require.Equal(t, expected, status)
	require.Equal(t, want[:end], calls)
	if fail == len(want)-2 && !strings.Contains(stderr.String(), "rewrite went unverified") {
		t.Fatal("missing downgrade warning")
	}
}

func refreshRunner(t *testing.T, calls *[]string, fail int) runner {
	t.Helper()
	return func(_ context.Context, name string, args []string, scrub bool, stdout, _ io.Writer) int {
		call := name + " " + strings.Join(args, " ")
		if scrub != (name == "vendir") {
			t.Fatalf("environment scrub for %s: %v", call, scrub)
		}
		*calls = append(*calls, call)
		if call == "mise tasks ls --all" {
			fmt.Fprintln(stdout, "devtools:check-pins updated payload")
		}
		if len(*calls)-1 == fail {
			return 19
		}
		return 0
	}
}

func TestUpdateStopsAtEachRewriteFailure(t *testing.T) {
	t.Parallel()
	for _, file := range []string{"apm.yml", "tombi.toml", "mise.toml", ".github/workflows/extra.yaml", ".github/renovate.json5", "vendir.yml", ".pre-commit-config.yaml"} {
		t.Run(file, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			fixture(t, dir, "annotated")
			require.NoError(t, os.Mkdir(filepath.Join(dir, file+".pins.tmp"), 0o700))
			var out, stderr bytes.Buffer
			var calls []string
			u := updater{dir: dir, out: &out, stderr: &stderr, run: resolutionRunner("annotated", &calls)}
			require.Equal(t, 1, u.update(t.Context(), "0.11.0"))
			require.Contains(t, stderr.String(), filepath.Base(file))
			for _, call := range calls {
				require.True(t, strings.HasPrefix(call, "git "), call)
			}
		})
	}
}
