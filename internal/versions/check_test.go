// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package versions_test

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/versions"
)

type site struct {
	file, text, want string
	line             int
}

func sites() []site {
	return []site{
		{"apm.yml", "version: 1.2.3", "version: 1.2.3", 2},
		{"packages/agents/common/apm.yml", "version: 1.2.3", "version: 1.2.3", 2},
		{"packages/agents/claude/apm.yml", "version: 1.2.3", "version: 1.2.3", 2},
		{"packages/agents/codex/apm.yml", "version: 1.2.3", "version: 1.2.3", 2},
		{"packages/agents/agy/apm.yml", "version: 1.2.3", "version: 1.2.3", 2},
		{"packages/devtools/pyproject.toml", `version = "1.2.3"`, `version = "1.2.3"`, 3},
		{".pre-commit-hooks.yaml", "#     rev: v1.2.3", "rev: v1.2.3", 3},
		{"README.md", "    apm install tbhb-dev/devtools#v1.2.3", "tbhb-dev/devtools#v1.2.3", 3},
		{"README.md", "    apm install tbhb-dev/devtools/packages/agents/claude#v1.2.3", "#v1.2.3", 5},
		{"README.md", "    rev: v1.2.3", "rev: v1.2.3", 9},
		{"uv.lock", `version = "1.2.3"`, `version = "1.2.3"`, 7},
	}
}

func write(t *testing.T, root, name, data string) {
	t.Helper()
	dir, err := os.OpenRoot(root)
	require.NoError(t, err)
	defer dir.Close()
	require.NoError(t, dir.MkdirAll(filepath.Dir(name), 0o700))
	require.NoError(t, dir.WriteFile(name, []byte(data), 0o600))
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "apm.yml", "name: devtools\nversion: 1.2.3\n")
	var lock strings.Builder
	// Reverse package order to ensure matching uses the local path, not position.
	for _, name := range []string{"agy", "codex", "claude", "common"} {
		write(t, root, "packages/agents/"+name+"/apm.yml", "name: "+name+"\nversion: 1.2.3\n")
		fmt.Fprintf(&lock,
			"- repo_url: _local/%s\n  version: 1.2.3\n  source: local\n  local_path: ./packages/agents/%s\n",
			name,
			name,
		)
	}
	write(t, root, "apm.lock.yaml", lock.String())
	write(t, root, "packages/devtools/pyproject.toml", "[project]\nname = \"devtools\"\nversion = \"1.2.3\"\n")
	write(t, root, ".pre-commit-hooks.yaml", "# Reference it with:\n#\n#     rev: v1.2.3\n")
	write(
		t,
		root,
		"README.md",
		"# devtools\n\n    apm install tbhb-dev/devtools#v1.2.3\n\n    apm install tbhb-dev/devtools/packages/agents/claude#v1.2.3\n\nAnd:\n\n    rev: v1.2.3\n",
	)
	write(
		t,
		root,
		"uv.lock",
		"[[package]]\nname = \"pytest\"\nversion = \"9.9.9\"\n\n[[package]]\nname = \"devtools\"\nversion = \"1.2.3\"\n",
	)
	git(t, root, "init", "--quiet", "--initial-branch=main")
	git(t, root, "add", "apm.yml")
	git(t, root, "commit", "--quiet", "-m", "fixture")
	return root
}

func gitEnv() []string {
	env := []string{}
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			env = append(env, value)
		}
	}
	return append(env, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir, cmd.Env = root, gitEnv()
	data, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", data)
}

func check(t *testing.T, root, version string, want int) string {
	t.Helper()
	var out, stderr bytes.Buffer
	require.Equal(
		t,
		want,
		versions.Run(t.Context(), root, version, &out, &stderr),
		"out %s; stderr %s",
		&out,
		&stderr,
	)
	require.Empty(t, stderr.String())
	return out.String()
}

func TestCleanAndExplicit(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	require.Empty(t, check(t, root, "1.2.3", 0), "AC1, AC4")
	for _, tag := range []string{"", "v9.9.9"} {
		if tag != "" {
			git(t, root, "tag", tag)
		}
		for _, version := range []string{"1.2.3", "v1.2.3"} {
			require.Empty(t, check(t, root, version, 0), "AC7")
		}
	}
}

func TestEverySiteStaleOrDeleted(t *testing.T) {
	t.Parallel()
	for _, s := range sites() {
		t.Run(fmt.Sprintf("%s:%d", s.file, s.line), func(t *testing.T) {
			t.Parallel()
			root := fixture(t)
			data, err := os.ReadFile(filepath.Join(root, s.file))
			require.NoError(t, err)
			stale := strings.ReplaceAll(s.text, "1.2.3", "1.2.2")
			write(t, root, s.file, strings.ReplaceAll(string(data), s.text, stale))
			want := fmt.Sprintf(
				"%s:%d [error] version-literal-stale  expected `%s`, found `%s`\n",
				s.file,
				s.line,
				s.want,
				stale,
			)
			total := 1
			if strings.HasPrefix(s.file, "packages/agents/") {
				// A stale manifest also disagrees with its local lock entry.
				total++
				want = fmt.Sprintf(
					"apm.lock.yaml:%d [error] version-literal-stale  %s expected `version: 1.2.2`, found `version: 1.2.3`\n",
					lockLine(s.file),
					strings.TrimSuffix(s.file, "/apm.yml"),
				) + want
			}
			want += fmt.Sprintf("TOTAL: %d finding(s) against v1.2.3\n", total)
			require.Equal(t, want, check(t, root, "1.2.3", 1), "AC2")
			write(t, root, s.file, strings.ReplaceAll(string(data), s.text+"\n", ""))
			output := check(t, root, "1.2.3", 1)
			require.Contains(t, output, s.file+":1 [error] version-literal-stale", "AC3")
			if s.file == "uv.lock" {
				require.Contains(t, output, "no package entry named devtools")
			} else {
				require.Contains(t, output, "no line matching")
			}
		})
	}
}

func lockLine(file string) int {
	for i, name := range []string{"agy", "codex", "claude", "common"} {
		if strings.Contains(file, "/"+name+"/") {
			return i*4 + 2
		}
	}
	return 0
}

func TestSimultaneousFindings(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	output := check(t, root, "9.9.9", 1)
	var want strings.Builder
	for _, s := range sites() {
		fmt.Fprintf(
			&want,
			"%s:%d [error] version-literal-stale  expected `%s`, found `%s`\n",
			s.file,
			s.line,
			strings.ReplaceAll(s.want, "1.2.3", "9.9.9"),
			s.text,
		)
	}
	want.WriteString("TOTAL: 11 finding(s) against v9.9.9\n")
	require.Equal(t, want.String(), output, "AC2: all findings in site order")
}

func TestLocks(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"common", "claude", "codex", "agy"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := fixture(t)
			data, err := os.ReadFile(filepath.Join(root, "apm.lock.yaml"))
			require.NoError(t, err)
			block := "- repo_url: _local/" + name + "\n  version: 1.2.3"
			write(
				t,
				root,
				"apm.lock.yaml",
				strings.ReplaceAll(string(data), block, strings.ReplaceAll(block, "1.2.3", "1.2.2")),
			)
			want := fmt.Sprintf(
				"apm.lock.yaml:%d [error] version-literal-stale  packages/agents/%s expected `version: 1.2.3`, found `version: 1.2.2`\nTOTAL: 1 finding(s) against v1.2.3\n",
				lockLine("/"+name+"/"),
				name,
			)
			require.Equal(t, want, check(t, root, "1.2.3", 1), "AC5")
			write(
				t,
				root,
				"apm.lock.yaml",
				strings.ReplaceAll(string(data), "  local_path: ./packages/agents/"+name, "  local_path: ./other"),
			)
			require.Equal(
				t,
				"apm.lock.yaml:1 [error] version-literal-stale  no local package entry for packages/agents/"+name+"\nTOTAL: 1 finding(s) against v1.2.3\n",
				check(t, root, "1.2.3", 1),
				"AC3, AC5",
			)
		})
	}
	root := fixture(t)
	write(t, root, "uv.lock", "[[package]]\nname = \"pytest\"\nversion = \"9.9.9\"\n")
	require.Equal(
		t,
		"uv.lock:1 [error] version-literal-stale  no package entry named devtools; the lockfile shape changed and the cog.toml hook that rewrites it is now inert\nTOTAL: 1 finding(s) against v1.2.3\n",
		check(t, root, "1.2.3", 1),
		"AC3",
	)
}

func TestLockScalarsIgnoreCommentsAndWhitespace(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	var lock strings.Builder
	for _, name := range []string{"agy", "codex", "claude", "common"} {
		write(t, root, "packages/agents/"+name+"/apm.yml", "name: "+name+"\nversion: 1.2.3 # release\n")
		fmt.Fprintf(
			&lock,
			"- repo_url: _local/%s\n  version:   1.2.3   \n  source: local\n"+
				"  local_path: ./packages/agents/%s \t# local\n",
			name,
			name,
		)
	}
	write(t, root, "apm.lock.yaml", lock.String())
	require.Empty(t, check(t, root, "1.2.3", 0), "AC1, AC5: comments and whitespace are not part of the value")

	write(t, root, "packages/agents/codex/apm.yml", "name: codex\nversion: 1.2.2 # release\n")
	out := check(t, root, "1.2.3", 1)
	require.Contains(
		t,
		out,
		"packages/agents/codex expected `version: 1.2.2`, found `version: 1.2.3`",
		"AC5: drift is still reported",
	)
}

func TestTagsAndShallowClone(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	git(t, root, "tag", "v9.9.9")
	git(t, root, "commit", "--quiet", "--allow-empty", "-m", "nearest")
	git(t, root, "tag", "v1.2.3")
	require.Empty(t, check(t, root, "", 0), "AC6: nearest tag, not highest version")
	git(t, root, "commit", "--quiet", "--allow-empty", "-m", "untagged head")
	shallow := filepath.Join(t.TempDir(), "shallow")
	remote := &url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(root), "/")}
	git(t, root, "clone", "--quiet", "--depth=1", remote.String(), shallow)
	assertNoTag(t, shallow)
	git(t, root, "tag", "-d", "v1.2.3", "v9.9.9")
	assertNoTag(t, root)
}

func assertNoTag(t *testing.T, root string) {
	t.Helper()
	var out, stderr bytes.Buffer
	require.Equal(t, 1, versions.Run(t.Context(), root, "", &out, &stderr))
	require.Empty(t, out.String())
	require.Equal(
		t,
		"check-versions: no tag reachable from HEAD; fetch tags and unshallow before running this\n",
		stderr.String(),
		"AC6",
	)
}

func TestReadFailures(t *testing.T) {
	t.Parallel()
	for _, file := range []string{"apm.yml", "packages/agents/common/apm.yml", "uv.lock", "apm.lock.yaml"} {
		t.Run(file, func(t *testing.T) {
			t.Parallel()
			root := fixture(t)
			require.NoError(t, os.Remove(filepath.Join(root, file)))
			require.Contains(
				t,
				check(t, root, "1.2.3", 1),
				file+":1 [error] version-literal-stale  cannot read file",
				"AC8",
			)
		})
	}
	root := fixture(t)
	file := filepath.Join(root, "README.md")
	// A directory at a site path makes ReadFile fail on every supported OS.
	require.NoError(t, os.Remove(file))
	require.NoError(t, os.Mkdir(file, 0o700))
	require.Contains(
		t,
		check(t, root, "1.2.3", 1),
		"README.md:1 [error] version-literal-stale  cannot read file",
		"AC8",
	)
	var out, stderr bytes.Buffer
	require.Equal(t, 1, versions.Run(t.Context(), t.TempDir(), "1.2.3", &out, &stderr))
	require.Empty(t, out.String())
	require.Contains(t, stderr.String(), "check-versions: git rev-parse", "AC8")
}
