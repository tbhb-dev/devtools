// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package hygiene_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/hygiene"
)

func write(t *testing.T, root, path, text string) {
	t.Helper()
	dir, err := os.OpenRoot(root)
	require.NoError(t, err)
	defer dir.Close()
	require.NoError(t, dir.MkdirAll(filepath.Dir(filepath.FromSlash(path)), 0o700))
	require.NoError(t, dir.WriteFile(filepath.FromSlash(path), []byte(text), 0o600))
}

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return string(out)
}

func TestCorpus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	git(t, root, "init", "--quiet")
	write(t, root, "sgconfig.yml", "ruleDirs: [rules]\n")
	for _, name := range []string{"bare-git-output", "diff-without-no-ext-diff", "ifs-command-substitution"} {
		data, err := os.ReadFile(filepath.Join("../../.ast-grep/rules", name+".yml"))
		require.NoError(t, err)
		write(t, root, "rules/"+name+".yml", string(data))
	}
	write(
		t,
		root,
		"tools/tracked.sh",
		"export LC_ALL=C\ngit log HEAD\ngitr diff 'flag --no-ext-diff'\nIFS=$(printf ' \\t\\n')\n",
	)
	write(t, root, "scripts/new.sh", "gh pr view\n")
	write(
		t,
		root,
		"hooks/clean.sh",
		"export LC_ALL=other\nunset GH_REPO\ngh pr view\ngitr diff --no-ext-diff\nIFS=$(printf '\\t')\necho 'git show'\n",
	)
	write(t, root, "packages/agents/codex/.apm/skills/published/scripts/a.sh", "git show HEAD\n")
	write(t, root, ".claude/skills/published/scripts/a.sh", "gh pr view\n")
	write(t, root, ".claude/skills/local/scripts/a.sh", "git status\n")
	write(t, root, "tools/ignored.sh", "gh pr view\n")
	write(t, root, "tools/exempt.sh", "# hygiene-exempt: fixture reason\ngh pr view\ngit log HEAD\n")
	write(t, root, ".gitignore", "tools/ignored.sh\n")
	git(t, root, "add", "--", "tools/tracked.sh")
	before := git(t, root, "status", "--porcelain=v1", "--untracked-files=all")
	index := git(t, root, "ls-files", "--stage")
	refs := git(t, root, "show-ref", "--head", "--exclude-existing")
	var out, stderr bytes.Buffer
	require.Equal(t, 1, hygiene.Run(t.Context(), root, nil, &out, &stderr))
	require.Empty(t, stderr.String())
	require.Contains(t, out.String(), "tools/tracked.sh:2 [error] bare-git-output")
	require.Contains(t, out.String(), "tools/tracked.sh:3 [error] diff-without-no-ext-diff")
	require.Contains(t, out.String(), "tools/tracked.sh:4 [error] ifs-command-substitution")
	require.Contains(t, out.String(), "write IFS=$' \\t\\n' instead")
	require.Contains(t, out.String(), "TOTAL: 8 finding(s)\n")
	require.NotContains(t, out.String(), "ignored.sh")
	require.NotContains(t, out.String(), "exempt.sh")
	require.NotContains(t, out.String(), ".claude/skills/published")
	require.NotContains(t, out.String(), "clean.sh")
	require.Equal(t, before, git(t, root, "status", "--porcelain=v1", "--untracked-files=all"))
	require.Equal(t, index, git(t, root, "ls-files", "--stage"))
	require.Equal(t, refs, git(t, root, "show-ref", "--head", "--exclude-existing"))
	out.Reset()
	require.Zero(
		t,
		hygiene.Run(t.Context(), root, []string{"missing", "hooks/clean.sh", "tools/exempt.sh"}, &out, &stderr),
	)
	require.Empty(t, out.String())
	require.Empty(t, stderr.String())
}
