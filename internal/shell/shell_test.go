// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package shell_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/shell"
)

// repoAllowlist is the allow list this repository publishes.
const repoAllowlist = "../../.devtools/shell-allowlist.toml"

func repoList(t *testing.T) *shell.Allowlist {
	t.Helper()
	list, err := shell.Load(repoAllowlist)
	require.NoError(t, err)
	return list
}

func check(t *testing.T, list *shell.Allowlist, src string) []shell.Finding {
	t.Helper()
	findings, err := list.Check("script.sh", strings.NewReader(src))
	require.NoError(t, err)
	return findings
}

func TestAllowedClasses(t *testing.T) {
	t.Parallel()
	list := repoList(t)
	classes := map[string][]string{
		"builtin": {
			"cd /tmp",
			"echo hi",
			"export X=1",
			"read -r line",
			"set -eu",
			"exit 1",
			"[ -f x ]",
			": noop",
			"source ./lib.sh",
			"trap true EXIT",
			"local x",
		},
		"tool":   {"git status", "gh pr view", "ssh host true", "scp a host:b", "mise run lint"},
		"binary": {"guard-markdown file.md", "gh-post --help", "agentrepo version", "shell-allowlist x.sh"},
		"coreutils": {
			"cat f",
			"cp a b",
			"mv a b",
			"rm f",
			"mkdir d",
			"chmod +x f",
			"chown u f",
			"ln -s a b",
			"install a b",
			"test -d d",
			"printf x",
			"date",
			"env",
			"uname",
			"hostname",
			"id",
			"sleep 1",
		},
	}
	for class, lines := range classes {
		for _, line := range lines {
			assert.Empty(t, check(t, list, line+"\n"), "%s: %s", class, line)
		}
	}
}

func TestEveryCmdDirectoryIsAllowed(t *testing.T) {
	t.Parallel()
	list := repoList(t)
	entries, err := os.ReadDir("../../cmd")
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() {
			assert.Empty(t, check(t, list, entry.Name()+"\n"), "cmd/%s", entry.Name())
		}
	}
}

func TestNotAllowed(t *testing.T) {
	t.Parallel()
	got := check(t, repoList(t), "git status\nbash -c true\nrsync a b\n")
	assert.Equal(t, []shell.Finding{
		{File: "script.sh", Line: 2, Command: "bash", Kind: shell.NotAllowed},
		{File: "script.sh", Line: 3, Command: "rsync", Kind: shell.NotAllowed},
	}, got)
	assert.Equal(t, "script.sh:2: bash: not on the allow list", got[0].String())
}

func TestBannedNames(t *testing.T) {
	t.Parallel()
	list := repoList(t)
	cases := map[string]string{
		"ruby":    "ruby -e 'puts 1'",
		"python":  "python -c 'print(1)'",
		"python3": "python3 -c 'print(1)'",
		"perl":    "perl -e 'print 1'",
		"node":    "node -e 'console.log(1)'",
		"jq":      "echo '{}' | jq .",
		"awk":     "awk '{print $1}' f",
		"sed":     "sed -i s/a/b/ f",
		"curl":    "curl https://example.com",
		"wget":    "wget https://example.com",
		"eval":    "eval \"$x\"",
	}
	for name, line := range cases {
		got := check(t, list, line+"\n")
		require.Len(t, got, 1, line)
		assert.Equal(
			t,
			shell.Finding{File: "script.sh", Line: 1, Command: name, Kind: shell.Banned},
			got[0],
			line,
		)
	}
}

func TestBannedHeredocInput(t *testing.T) {
	t.Parallel()
	src := "python3 <<EOF\nprint(1)\nEOF\n"
	got := check(t, repoList(t), src)
	assert.Equal(t, []shell.Finding{
		{File: "script.sh", Line: 1, Command: "python3", Kind: shell.Banned},
	}, got)
}

func TestHeredocFedToCatIsNotInspected(t *testing.T) {
	t.Parallel()
	list := repoList(t)
	assert.Empty(t, check(t, list, "cat <<'EOF'\npython3 -c x\njq . | sed s/a/b/\nEOF\n"))
	assert.Empty(t, check(t, list, "cat <<EOF\nrun curl here\nEOF\n"))
	assert.Empty(t, check(t, list, "cat <<EOF\n$(python3)\nEOF\n"))
}

func TestBannedWhereverItAppears(t *testing.T) {
	t.Parallel()
	list := repoList(t)
	for _, src := range []string{
		"x=$(jq . f)",
		"echo `jq . f`",
		"f() { jq . f; }\nf",
		"if true; then jq .; fi",
		"cat f | jq .",
		"echo hi && curl x",
		"diff <(sed p f) g",
		"/usr/bin/jq .",
		"./vendor/bin/python3 x",
		"\\jq .",
		"'jq' .",
		"\"jq\" .",
		"FOO=1 jq .",
	} {
		got := check(t, list, src+"\n")
		var banned int
		for _, finding := range got {
			if finding.Kind == shell.Banned {
				banned++
			}
		}
		assert.Equal(t, 1, banned, src)
	}
}

func TestUnresolvable(t *testing.T) {
	t.Parallel()
	list := repoList(t)
	for src, command := range map[string]string{
		"$cmd arg":         "$cmd",
		"${cmd} arg":       "${cmd}",
		"$(echo jq) arg":   "$(echo jq)",
		"\"$cmd\" arg":     "\"$cmd\"",
		"/opt/$tool/run x": "/opt/$tool/run",
	} {
		got := check(t, list, src+"\n")
		require.Len(t, got, 1, src)
		assert.Equal(t, shell.Unresolvable, got[0].Kind, src)
		assert.Equal(t, command, got[0].Command, src)
	}
}

func TestFunctionsDefinedInTheFileAreAllowed(t *testing.T) {
	t.Parallel()
	assert.Empty(t, check(t, repoList(t), "helper() { echo hi; }\nhelper\n"))
	got := check(t, repoList(t), "helper() { echo hi; }\nother\n")
	require.Len(t, got, 1)
	assert.Equal(t, "other", got[0].Command)
}

func TestBareAssignmentRunsNoCommand(t *testing.T) {
	t.Parallel()
	assert.Empty(t, check(t, repoList(t), "x=1\ny=\"$x\"\n"))
}

func TestBannedNameCannotBeAllowed(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "allow.toml")
	require.NoError(t, os.WriteFile(path, []byte("commands = [\"jq\", \"git\"]\n"), 0o600))
	list, err := shell.Load(path)
	require.NoError(t, err)
	got := check(t, list, "jq .\n")
	require.Len(t, got, 1)
	assert.Equal(t, shell.Banned, got[0].Kind)
}

func TestCheckRejectsInvalidShell(t *testing.T) {
	t.Parallel()
	_, err := repoList(t).Check("bad.sh", strings.NewReader("if then\n"))
	require.ErrorContains(t, err, "parsing shell")
	assert.Contains(t, err.Error(), "bad.sh")
}

func TestLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		return path
	}

	list, err := shell.Load(write("ok.toml", "commands = [\"echo\", \"git\"]\n"))
	require.NoError(t, err)
	assert.Empty(t, check(t, list, "echo a; git b\n"))
	assert.Len(t, check(t, list, "cat a\n"), 1)

	_, err = shell.Load(filepath.Join(dir, "missing.toml"))
	require.ErrorContains(t, err, "reading")

	_, err = shell.Load(write("typo.toml", "command = [\"echo\"]\n"))
	require.ErrorContains(t, err, "parsing")
	require.ErrorContains(t, err, "typo.toml")

	_, err = shell.Load(write("broken.toml", "commands = \n"))
	require.ErrorContains(t, err, "parsing")
}

func TestFunctionBindingsRespectOrderAndScope(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"rsync a b; rsync() { :; }",
		"( rsync() { :; } ); rsync a b",
		"if false; then rsync() { :; }; fi; rsync a b",
		"outer() { rsync() { :; }; }; rsync a b",
		"rsync() { :; }; unset -f rsync; rsync a b",
	} {
		t.Run(src, func(t *testing.T) {
			t.Parallel()
			got := check(t, repoList(t), src)
			assert.Equal(t, []shell.Finding{
				{File: "script.sh", Line: 1, Command: "rsync", Kind: shell.NotAllowed},
			}, got)
		})
	}
}

func TestCommandQuoteContexts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src     string
		command string
		kind    shell.Kind
	}{
		{`"g\it" status`, `g\it`, shell.NotAllowed},
		{`g\\it status`, `g\it`, shell.NotAllowed},
		{`$'\x6a\x71' .`, "jq", shell.Banned},
		{`'g\it' status`, `g\it`, shell.NotAllowed},
		{`$'\cA' status`, `$'\cA'`, shell.Unresolvable},
		{`$"git" status`, `$"git"`, shell.Unresolvable},
	} {
		t.Run(tc.src, func(t *testing.T) {
			t.Parallel()
			got := check(t, repoList(t), tc.src)
			assert.Equal(t, []shell.Finding{
				{File: "script.sh", Line: 1, Command: tc.command, Kind: tc.kind},
			}, got)
		})
	}
	for _, src := range []string{`g\it status`, `"git" status`, `$'git' status`} {
		t.Run(src, func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, check(t, repoList(t), src), src)
		})
	}
}

func TestGlobCommandsAreUnresolvable(t *testing.T) {
	t.Parallel()
	for _, word := range []string{"git*", "gi?", "./gi[t]", `./gi[t"t"]`, `"git"*`, "~user/git"} {
		assert.Equal(t, []shell.Finding{
			{File: "script.sh", Line: 1, Command: word, Kind: shell.Unresolvable},
		}, check(t, repoList(t), word+" status"))
	}
	for _, word := range []string{`'git*'`, `"git*"`, `git\*`} {
		got := check(t, repoList(t), word+" status")
		require.Len(t, got, 1)
		assert.Equal(t, "git*", got[0].Command)
		assert.Equal(t, shell.NotAllowed, got[0].Kind)
	}
}

func TestFunctionBindingsWithinSequentialScopes(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"{ helper() { :; }; helper; }",
		"( helper() { :; }; helper )",
		"echo $(helper() { :; }; helper)",
		"cat <(helper() { :; }; helper)",
		"helper() { :; }; echo ok; helper",
		"helper() { :; } >out; helper",
	} {
		assert.Empty(t, check(t, repoList(t), src), src)
	}
}

func TestFunctionBindingsStayUncertainAfterChanges(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"rsync() { :; } & rsync a b",
		"rsync() { :; }; source lib.sh; rsync a b",
		"rsync() { :; }; helper() { unset -f rsync; }; helper; rsync a b",
		"if false; then { rsync() { :; }; }; else rsync a b; fi",
		"rsync() { :; }; if true; then unset -f rsync; fi; rsync a b",
	} {
		assert.Equal(t, []shell.Finding{
			{File: "script.sh", Line: 1, Command: "rsync", Kind: shell.NotAllowed},
		}, check(t, repoList(t), src), src)
	}
}

func TestNonCatHeredocSubstitutions(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []shell.Finding{
		{File: "script.sh", Line: 2, Command: "python3", Kind: shell.Banned},
	}, check(t, repoList(t), "printf x <<EOF\n$(python3)\nEOF\n"))
}
