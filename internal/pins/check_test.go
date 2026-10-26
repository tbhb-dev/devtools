// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package pins_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/pins"
)

const tools = "[tools]\n\"go:github.com/tbhb-dev/devtools/cmd/guard-markdown\" = \"v0.10.0\"\n\"go:github.com/tbhb-dev/devtools/cmd/commit-lint\" = \"v0.10.0\"\n\"go:github.com/tbhb-dev/devtools/cmd/gh-post\" = \"v0.10.0\"\n"

func write(t *testing.T, root, name, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = root
	data, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", data)
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q")
	write(t, root, "mise.toml", tools)
	git(t, root, "add", "--", "mise.toml")
	return root
}

type result struct {
	status      int
	out, stderr string
}

func check(t *testing.T, root string) result {
	t.Helper()
	var out, stderr bytes.Buffer
	status := pins.Run(t.Context(), root, &out, &stderr)
	return result{status, out.String(), stderr.String()}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, file, data, diagnostic string
		status                       int
	}{
		{"required tools only", "", "", "every devtools pin is on v0.10.0", 0},
		{"root APM", "apm.yml", "  - tbhb-dev/devtools#v0.10.0\n", "apm.yml:1", 0},
		{"quoted packages", "apm.yml", " - 'tbhb-dev/devtools/packages/agents/common#v0.10.0'\n - \"tbhb-dev/devtools/packages/agents/claude#v0.10.0\"\n - tbhb-dev/devtools/packages/agents/codex#v0.10.0\n - tbhb-dev/devtools/packages/agents/agy#v0.10.0\n", "apm.yml:4", 0},
		{"unsupported package", "apm.yml", " - tbhb-dev/devtools/packages/unknown#v0.10.0\n", "pins disagree on the version", 1},
		{"unknown version", "apm.yml", " - tbhb-dev/devtools#main\n", "  ?\n", 1},
		{"version drift", "apm.yml", " - tbhb-dev/devtools#v0.9.0\n", "  v0.9.0\n    apm.yml:1", 1},
		{"vendir", "vendir.yml", "url: https://github.com/tbhb-dev/devtools\nref: v0.10.0\n", "vendir.yml:2", 0},
		{"precommit", ".pre-commit-config.yaml", "repo: https://github.com/tbhb-dev/devtools\nrev: abc # frozen: v0.10.0\n", "abc", 0},
		{"missing frozen comment", ".pre-commit-config.yaml", "repo: https://github.com/tbhb-dev/devtools\nrev: abc\n", "pins disagree on the version", 1},
		{"workflow yml", ".github/workflows/a.yml", "uses: tbhb-dev/devtools/.github/workflows/lint.yml@abc # v0.10.0\n", ".github/workflows/a.yml:1", 0},
		{"workflow yaml", ".github/workflows/b.yaml", "uses: tbhb-dev/devtools/.github/workflows/lint.yml@abc # v0.10.0\n", ".github/workflows/b.yaml:1", 0},
		{"missing workflow comment", ".github/workflows/a.yml", "uses: tbhb-dev/devtools/.github/workflows/lint.yml@abc\n", "pins disagree on the version", 1},
		{"tag preset", ".github/renovate.json5", "\"github>tbhb-dev/devtools//.github/custom.json5#v0.10.0\"\n", ".github/renovate.json5:1", 0},
		{"sha presets", ".github/renovate.json5", "\"github>tbhb-dev/devtools//.github/custom.json5#abc\", // v0.10.0\n\"github>tbhb-dev/devtools//.github/other.json5#def\", // v0.10.0\n", "pins disagree on the commit", 1},
		{"unpinned preset", ".github/renovate.json5", "\"github>tbhb-dev/devtools//.github/custom.json5\"\n", "unpinned", 1},
		{"missing preset comment", ".github/renovate.json5", "\"github>tbhb-dev/devtools//.github/custom.json5#abc\"\n", "pins disagree on the version", 1},
		{"guard drift", "mise.toml", strings.Replace(tools, "v0.10.0", "v0.9.0", 1), "pins disagree on the version", 1},
		{"commit lint drift", "mise.toml", strings.Replace(tools, `cmd/commit-lint" = "v0.10.0`, `cmd/commit-lint" = "v0.9.0`, 1), "pins disagree on the version", 1},
		{"gh post drift", "mise.toml", strings.Replace(tools, `cmd/gh-post" = "v0.10.0`, `cmd/gh-post" = "v0.9.0`, 1), "pins disagree on the version", 1},
		{"agentrepo drift", "mise.toml", tools + "\"go:github.com/tbhb-dev/devtools/cmd/agentrepo\" = \"v0.9.0\"\n", "pins disagree on the version", 1},
		{"agentrepo present", "mise.toml", tools + "\"go:github.com/tbhb-dev/devtools/cmd/agentrepo\" = \"v0.10.0\"\n", "every devtools pin", 0},
		{"missing guard", "mise.toml", strings.ReplaceAll(tools, "guard-markdown", "agentrepo"), "missing guard-markdown", 1},
		{"missing commit lint", "mise.toml", strings.ReplaceAll(tools, "cmd/commit-lint", "cmd/other"), "missing commit-lint", 1},
		{"missing gh post", "mise.toml", strings.ReplaceAll(tools, "cmd/gh-post", "cmd/other"), "missing gh-post", 1},
		{"matching prose", "tombi.toml", "# tbhb-dev/devtools v0.10.0.\n", "every devtools pin", 0},
		{"multiple stale mentions", "tombi.toml", "# tbhb-dev/devtools v0.9.0 and tbhb-dev/devtools v0.8.0.\n", "tombi.toml:1: prose mentions v0.8.0; pinned v0.10.0 (edit manually)", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := fixture(t)
			if tc.file != "" {
				write(t, root, tc.file, tc.data)
				git(t, root, "add", "--", tc.file)
			}
			got := check(t, root)
			require.Equal(t, tc.status, got.status, "%s%s", got.out, got.stderr)
			require.Contains(t, got.out+got.stderr, tc.diagnostic)
			compareShell(t, root, got)
		})
	}
}

func TestWorkingTreeProse(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	write(t, root, "tracked.md", "tbhb-dev/devtools v0.10.0\n")
	git(t, root, "add", "--", "tracked.md")
	write(t, root, "untracked.md", "tbhb-dev/devtools v0.1.0\n")
	require.Zero(t, check(t, root).status)
	write(t, root, "tracked.md", "tbhb-dev/devtools v0.9.0\n")
	got := check(t, root)
	require.Equal(t, 1, got.status)
	require.Contains(t, got.stderr, "tracked.md:1: prose mentions v0.9.0")
	require.NotContains(t, got.stderr, "untracked")
	compareShell(t, root, got)
}

func TestReadOnly(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	cmd := exec.CommandContext(t.Context(), "git", "status", "--porcelain=v1")
	cmd.Dir = root
	before, err := cmd.Output()
	require.NoError(t, err)
	require.Zero(t, check(t, root).status)
	cmd = exec.CommandContext(t.Context(), "git", "status", "--porcelain=v1")
	cmd.Dir = root
	after, err := cmd.Output()
	require.NoError(t, err)
	require.Equal(t, before, after)
	data, err := os.ReadFile(filepath.Join(root, "mise.toml"))
	require.NoError(t, err)
	require.Equal(t, tools, string(data))
}

func TestFrozenFallback(t *testing.T) {
	t.Parallel()
	frozen, err := os.ReadFile("../../.devtools/scripts/check-pins-legacy.sh")
	require.NoError(t, err)
	require.Equal(
		t,
		"a159800d02585234e444f51eb0a4df2fac5fa6016df5cbb1dcb784740cf601ba",
		fmt.Sprintf("%x", sha256.Sum256(frozen)),
	)
}
