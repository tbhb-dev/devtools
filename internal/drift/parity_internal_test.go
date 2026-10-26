//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package drift

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func worktree(t *testing.T, dir string) map[string]string {
	t.Helper()
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer root.Close()
	files := map[string]string{}
	require.NoError(t, fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk fixture: %w", err)
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		data, err := root.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read fixture: %w", err)
		}
		files[path] = string(data)
		return nil
	}))
	return files
}

func TestFrozenShellParity(t *testing.T) {
	t.Setenv("PATH", os.Getenv("PATH"))
	for _, vendir := range []bool{false, true} {
		for _, scenario := range []string{"clean", "modified", "deleted", "new", "staged", "ignored", "personal", "regenerated", "tidy-failure", "vendor-failure", "sync-failure", "git-manifest", "git-diff", "git-listing", "glob", "unicode-space"} {
			t.Run(scenario, func(t *testing.T) {
				t.Setenv("PATH", os.Getenv("PATH"))
				dir := fixture(t)
				command, path := "vendor-check", "vendor/tracked"
				if vendir {
					command, path = "check-vendored", ".devtools/scripts/tracked"
				}
				prepareParity(t, dir, path, scenario)
				stubTools(t, path, scenario)
				frozen, err := filepath.Abs(filepath.Join("testdata", command+".txt"))
				require.NoError(t, err)
				fallback, err := filepath.Abs(
					filepath.Join("..", "..", ".devtools", "scripts", "legacy", command+".sh"),
				)
				require.NoError(t, err)
				t.Chdir(dir)
				before := snapshot(t, dir)
				shellCode, shellOut, shellErr := shellResult(t, frozen)
				shellFiles := worktree(t, dir)
				if scenario == "regenerated" {
					write(t, dir, path, "tracked\n")
				}
				fallbackCode, fallbackOut, fallbackErr := shellResult(t, fallback)
				require.Equal(t, shellCode, fallbackCode)
				require.Equal(t, shellOut, fallbackOut)
				require.Equal(t, shellErr, fallbackErr)
				if scenario == "regenerated" {
					write(t, dir, path, "tracked\n")
				}
				var out, stderr bytes.Buffer
				code := Run(t.Context(), vendir, &out, &stderr)
				require.Equal(t, shellCode, code)
				require.Equal(t, shellOut, out.String())
				require.Equal(t, shellErr, stderr.String())
				require.Equal(t, shellFiles, worktree(t, dir))
				require.Equal(t, before, snapshot(t, dir))
			})
		}
	}
}

func prepareParity(t *testing.T, dir, path, scenario string) {
	t.Helper()
	switch scenario {
	case "glob":
		write(t, dir, filepath.Join(filepath.Dir(path), "*"), "new\n")
	case "unicode-space":
		git(t, dir, "config", "core.quotePath", "false")
		write(t, dir, path+"\u00a0name", "new\n")
	case "modified":
		write(t, dir, path, "modified\n")
	case "deleted":
		require.NoError(t, os.Remove(filepath.Join(dir, path)))
	case "new":
		write(t, dir, path+"-new", "new\n")
	case "staged":
		write(t, dir, path, "staged\n")
		git(t, dir, "add", "--", path)
	case "ignored":
		write(t, dir, filepath.Join(filepath.Dir(path), "ignored"), "ignored\n")
	case "personal":
		write(t, dir, path+"-hidden", "hidden\n")
		write(t, dir, ".git/info/exclude", "*-hidden\n")
	}
}

func stubTools(t *testing.T, path, scenario string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	t.Setenv("REAL_GIT", realGit)
	t.Setenv("FAIL_GIT", strings.TrimPrefix(scenario, "git-"))
	bin := t.TempDir()
	script := "#!/usr/bin/env bash\necho \"$*\"\necho \"diagnostic $*\" >&2\n"
	for _, tool := range []string{"go", "vendir"} {
		body := script
		if scenario == "regenerated" {
			body += "echo regenerated >" + path + "\n"
		}
		if scenario == "sync-failure" && tool == "vendir" {
			body += "exit 27\n"
		}
		if tool == "go" && strings.HasSuffix(scenario, "-failure") && scenario != "sync-failure" {
			body += "if [ \"$2\" = " + strings.TrimSuffix(scenario, "-failure") + " ]; then exit 23; fi\n"
		}
		root, openErr := os.OpenRoot(bin)
		require.NoError(t, openErr)
		require.NoError(t, root.WriteFile(tool, []byte(body), 0o700))
		require.NoError(t, root.Close())
	}
	root, err := os.OpenRoot(bin)
	require.NoError(t, err)
	gitStub := `#!/usr/bin/env bash
operation=$1
case "$1:$2" in
	ls-files:--others) operation=listing ;;
	ls-files:--) operation=manifest ;;
esac
if [ "$operation" = "$FAIL_GIT" ]; then
	echo partial
	echo 'git failed' >&2
	exit 42
fi
exec "$REAL_GIT" "$@"
`
	require.NoError(t, root.WriteFile("git", []byte(gitStub), 0o700))
	require.NoError(t, root.Close())
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func shellResult(t *testing.T, script string) (int, string, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	cmd := exec.CommandContext(t.Context(), "bash", script)
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		code = cmd.ProcessState.ExitCode()
	}
	return code, out.String(), stderr.String()
}
