// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestConfigCommands(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	git := exec.CommandContext(t.Context(), "git", "init", "--quiet")
	if data, err := git.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, data)
	}
	machineDir := filepath.Join(root, "config", "devtools")
	repoDir := filepath.Join(root, ".agents")
	for _, dir := range []string{machineDir, repoDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(machineDir, "devtools.conf"),
		[]byte("unit_value = literal\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	repoFile := filepath.Join(repoDir, "agent-skills.conf")
	if err := os.WriteFile(repoFile, []byte("hook_marker = marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		kind, key string
		fallback  []string
		want      string
	}{
		{kind: "machine", key: "unit_value", want: "literal\n"},
		{kind: "machine", key: "unit_missing", want: "\n"},
		{kind: "machine", key: "unit_missing", fallback: []string{"default"}, want: "default\n"},
		{kind: "repository", key: "hook_marker", want: "marker\n"},
		{kind: "repository", key: "required_hooks", fallback: []string{"fallback"}, want: "fallback\n"},
	} {
		var out, stderr bytes.Buffer
		args := append([]string{"config", tc.kind, tc.key}, tc.fallback...)
		if code := agentrepo.Run(args, nil, &out, &stderr); code != 0 || out.String() != tc.want || stderr.Len() != 0 {
			t.Fatalf("%v: code %d, %q, %q", args, code, &out, &stderr)
		}
	}
	if err := os.WriteFile(repoFile, []byte("review_cmd = planted\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := agentrepo.Run(
		[]string{"config", "repository", "hook_marker"},
		nil,
		&out,
		&stderr,
	); code != 1 || out.Len() != 0 ||
		!strings.Contains(stderr.String(), "Invalid or duplicate repository setting: review_cmd") {
		t.Fatalf("code %d, %q, %q", code, &out, &stderr)
	}
}

func TestConfigOutsideRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DEVTOOLS_REVIEW_CMD", "")
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{args: []string{"config", "machine", "review_cmd", "fallback"}, want: "\n"},
		{args: []string{"config", "machine", "unit_missing", "fallback"}, code: 128},
		{args: []string{"config", "repository", "hook_marker", "fallback"}, code: 128},
	} {
		var out, stderr bytes.Buffer
		if code := agentrepo.Run(tc.args, nil, &out, &stderr); code != tc.code || out.String() != tc.want {
			t.Fatalf("%v: code %d, %q, %q", tc.args, code, &out, &stderr)
		}
	}
}
