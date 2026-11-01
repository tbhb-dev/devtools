// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//nolint:paralleltest // Tests change the process working directory to select the allow list.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func script(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestAllowedClasses(t *testing.T) {
	t.Chdir("../..")
	classes := map[string]string{
		"builtins":            ": . alias bg bind break builtin caller cd command compgen complete compopt continue declare dirs disown echo enable exec exit export false fc fg getopts hash help history jobs kill let local logout mapfile popd printf pushd pwd read readarray readonly return set shift shopt source suspend test times trap true type typeset ulimit umask unalias unset wait [",
		"tools":               "git gh ssh scp mise",
		"repository binaries": "agentcontext agenthooks agentrepo agentstore agy-otel commit-lint gh-post gh-reviewer guard-markdown shell-allowlist ssh-sign test-hooks",
		"coreutils":           "cat cp mv rm mkdir chmod chown ln install test printf date env uname hostname id sleep",
	}
	for class, names := range classes {
		t.Run(class, func(t *testing.T) {
			var out bytes.Buffer
			input := strings.ReplaceAll(names, " ", "\n") + "\n"
			input = strings.ReplaceAll(input, "\nlet\n", "\nlet 1\n")
			path := script(t, "input", input)
			assert.Equal(t, 0, Run([]string{path}, &out))
			assert.Empty(t, out.String())
		})
	}
}

func TestReports(t *testing.T) {
	t.Chdir("../..")
	banned := []string{"ruby", "python", "python3", "perl", "node", "jq", "awk", "sed", "curl", "wget", "eval"}
	for _, name := range banned {
		t.Run(name, func(t *testing.T) {
			for _, input := range []string{name + "\n", "git status | " + name + " .\n", name + " <<EOF\nbody\nEOF\n"} {
				var out bytes.Buffer
				path := script(t, "input", "# heading\n"+input)
				assert.Equal(t, 1, Run([]string{path}, &out))
				assert.Equal(t, fmt.Sprintf("%s:2: %s: banned\n", path, name), out.String())
			}
		})
	}
	cases := []struct{ input, report string }{
		{"python3 -c 'print(1)'", "python3: banned"},
		{"ruby -e 'puts 1'", "ruby: banned"},
		{"node -e 'console.log(1)'", "node: banned"},
		{"perl -e 'print 1'", "perl: banned"},
		{"unknown-tool", "unknown-tool: not on the allow list"},
		{"$cmd", "$cmd: unresolvable"},
		{"\"${cmd}\"", "\"${cmd}\": unresolvable"},
		{"$(printf git)", "$(printf git): unresolvable"},
		{"`printf git`", "$(printf git): unresolvable"},
		{"g${suffix}", "g${suffix}: unresolvable"},
		{"printf '%s' \"$(jq .)\"", "jq: banned"},
		{"value=$(curl url)", "curl: banned"},
		{"(sed x)", "sed: banned"},
		{"if true; then awk x; fi", "awk: banned"},
		{"f() { node; }", "node: banned"},
		{"'python3' -c x", "python3: banned"},
		{"\"python3\" -c x", "python3: banned"},
		{"p\\ython3 -c x", "python3: banned"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			var out bytes.Buffer
			path := script(t, "input", tc.input+"\n")
			assert.Equal(t, 1, Run([]string{path}, &out))
			assert.Equal(t, path+":1: "+tc.report+"\n", out.String())
		})
	}
}

func TestNamedFilesOnly(t *testing.T) {
	t.Chdir("../..")
	bad := script(t, "ignored.sh", "ruby\n")
	good := filepath.Join(filepath.Dir(bad), "no-extension")
	require.NoError(t, os.WriteFile(good, []byte("git status\n"), 0o600))
	var out bytes.Buffer
	assert.Equal(t, 0, Run([]string{good}, &out))
	assert.Empty(t, out.String())
	assert.Equal(t, 1, Run([]string{bad}, &out))
	assert.Equal(t, bad+":1: ruby: banned\n", out.String())
	out.Reset()
	assert.Equal(t, 0, Run(nil, &out))
	assert.Empty(t, out.String())
	out.Reset()
	assert.Equal(t, 1, Run([]string{filepath.Dir(bad)}, &out))
	assert.NotContains(t, out.String(), "ruby")
}

func TestAllReportsAndFiles(t *testing.T) {
	t.Chdir("../..")
	first := script(t, "first", "ruby\nnode | jq .\n")
	second := script(t, "second", "curl url\n")
	var out bytes.Buffer
	assert.Equal(t, 1, Run([]string{first, second}, &out))
	assert.Equal(
		t,
		first+":1: ruby: banned\n"+first+":2: node: banned\n"+first+":2: jq: banned\n"+second+":1: curl: banned\n",
		out.String(),
	)
}

func TestLiteralData(t *testing.T) {
	t.Chdir("../..")
	path := script(
		t,
		"input",
		"cat <<EOF\nruby\n$(python3)\nEOF\nprintf '%s' 'node'\n'git' status\n\"git\" status\ng\\it status\n",
	)
	var out bytes.Buffer
	assert.Equal(t, 0, Run([]string{path}, &out))
	assert.Empty(t, out.String())
}

func TestNonCatHeredocSubstitutions(t *testing.T) {
	t.Chdir("../..")
	path := script(t, "input", "printf x <<EOF\n$(python3 -c 'print(1)')\nEOF\n")
	var out bytes.Buffer
	assert.Equal(t, 1, Run([]string{path}, &out))
	assert.Equal(t, path+":2: python3: banned\n", out.String())
}

func TestReadsAllowlist(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".devtools"), 0o700))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(dir, ".devtools", "shell-allowlist.toml"), []byte("commands = ['cat']\n"), 0o600),
	)
	path := script(t, "input", "cat file\ngit status\n")
	t.Chdir(dir)
	var out bytes.Buffer
	assert.Equal(t, 1, Run([]string{path}, &out))
	assert.Equal(t, path+":2: git: not on the allow list\n", out.String())
}

func TestInputErrors(t *testing.T) {
	t.Chdir("../..")
	for _, path := range []string{filepath.Join(t.TempDir(), "missing"), script(t, "bad", "if\n")} {
		var out bytes.Buffer
		assert.Equal(t, 1, Run([]string{path}, &out))
		assert.Contains(t, out.String(), path)
	}
}

func TestLibraryCommandChecks(t *testing.T) {
	t.Chdir("../..")
	for _, tc := range []struct{ input, report string }{
		{"git* status", "git*: unresolvable"},
		{`"g\it" status`, `g\it: not on the allow list`},
		{"rsync a b; rsync() { :; }", "rsync: not on the allow list"},
		{"(rsync() { :; }); rsync a b", "rsync: not on the allow list"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			var out bytes.Buffer
			path := script(t, "input", tc.input+"\n")
			assert.Equal(t, 1, Run([]string{path}, &out))
			assert.Equal(t, path+":1: "+tc.report+"\n", out.String())
		})
	}
}
