// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoAllowlist is the allow list this repository publishes.
const repoAllowlist = "../../.devtools/shell-allowlist.toml"

type result struct {
	code   int
	stdout string
	stderr string
}

func run(args ...string) result {
	var out, errOut bytes.Buffer
	code := Run(append([]string{"--allowlist", repoAllowlist}, args...), streams{out: &out, err: &errOut})
	return result{code: code, stdout: out.String(), stderr: errOut.String()}
}

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestCleanScriptPrintsNothingAndExitsZero(t *testing.T) {
	t.Parallel()
	script := write(
		t,
		t.TempDir(),
		"clean.sh",
		"#!/usr/bin/env bash\nset -eu\ngit status\ncat <<'EOF'\npython3 in text\nEOF\n",
	)
	got := run(script)
	assert.Equal(t, 0, got.code)
	assert.Empty(t, got.stdout)
	assert.Empty(t, got.stderr)
}

func TestReportsFileLineAndCommandAndExitsOne(t *testing.T) {
	t.Parallel()
	script := write(t, t.TempDir(), "bad.sh", "git status\npython3 -c 'print(1)'\n$cmd run\nrsync a b\n")
	got := run(script)
	assert.Equal(t, 1, got.code)
	assert.Equal(t, script+":2: python3: banned\n"+
		script+":3: $cmd: unresolvable\n"+
		script+":4: rsync: not on the allow list\n", got.stdout)
	assert.Empty(t, got.stderr)
}

func TestChecksExactlyTheNamedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A shebang and a .sh name make a file look like shell, and neither one
	// gets it checked unless the caller names it.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o700))
	write(t, dir, "ignored.sh", "#!/bin/bash\njq .\n")
	write(t, dir, "sub/ignored.sh", "jq .\n")

	clean := write(t, dir, "clean.sh", "git status\n")
	got := run(clean)
	assert.Equal(t, 0, got.code, got.stdout)

	// Whatever the name, a named file is checked.
	noExt := write(t, dir, "Makefile.in", "curl x\n")
	got = run(clean, noExt)
	assert.Equal(t, 1, got.code)
	assert.Equal(t, noExt+":1: curl: banned\n", got.stdout)
}

func TestDirectoryArgumentIsAnError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "inside.sh", "jq .\n")
	got := run(dir)
	assert.Equal(t, 2, got.code)
	assert.Empty(t, got.stdout)
	assert.Contains(t, got.stderr, "shell-allowlist:")
}

func TestNoFilesIsClean(t *testing.T) {
	t.Parallel()
	got := run()
	assert.Equal(t, 0, got.code)
	assert.Empty(t, got.stdout)
}

func TestErrorsExitTwo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	parseFail := write(t, dir, "broken.sh", "if then\n")
	for name, args := range map[string][]string{
		"missing file":  {filepath.Join(dir, "absent.sh")},
		"invalid shell": {parseFail},
	} {
		got := run(args...)
		assert.Equal(t, 2, got.code, name)
		assert.Contains(t, got.stderr, "shell-allowlist:", name)
	}

	var out, errOut bytes.Buffer
	code := Run([]string{"--allowlist", filepath.Join(dir, "nope.toml"), parseFail}, streams{out: &out, err: &errOut})
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut.String(), "allow list")
}
