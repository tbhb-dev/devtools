// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package config_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMergeMachineRefusesEnvironment(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"merge_owner", "delegation_check", "required_reviewer", "required_reviewer_type", "gh_login"} {
		for _, value := range []string{"override", ""} {
			t.Run(key+"/"+value, func(t *testing.T) {
				t.Parallel()
				name := "DEVTOOLS_" + strings.ToUpper(key)
				f := newFixture(t, configCase{machine: key + " = machine\n", env: map[string]string{name: value}})
				f.lookup.RepositoryName = func() (string, error) { return "", errDiscovery }
				// Refuse every authority override, even when reading a different key.
				got, err := f.lookup.MergeMachine("gh_login", "fallback")
				if got != "" || err == nil || !strings.Contains(err.Error(), name) {
					t.Fatalf("%q, %v", got, err)
				}
				got, err = f.lookup.Machine(key, "fallback")
				if err != nil || got != value {
					t.Fatalf("ordinary lookup: %q, %v", got, err)
				}
			})
		}
	}
}

func TestMergeMachineFiles(t *testing.T) {
	t.Parallel()
	for _, tc := range []configCase{
		{name: "repository file", key: "merge_owner", machine: "merge_owner = repo\n", defaults: "merge_owner = shared\n", want: "repo"},
		{name: "shared file", key: "required_reviewer", defaults: "required_reviewer = shared\n", want: "shared"},
		{name: "fallback", key: "gh_login", fallback: "default", want: "default"},
		{name: "empty", key: "delegation_check", machine: "delegation_check =\n", fallback: "fallback"},
		{name: "HOME fallback", key: "gh_login", defaults: "gh_login = shared\n", home: true, want: "shared"},
		{name: "repository cannot grant authority", key: "gh_login", repository: "gh_login = planted\n", fallback: "fallback", want: "fallback"},
		{name: "literal", key: "delegation_check", machine: "delegation_check = $(literal) # value\n", want: "$(literal) # value"},
		{name: "other environment ignored", key: "review_cmd", machine: "review_cmd = machine\n", env: map[string]string{"DEVTOOLS_REVIEW_CMD": "override"}, want: "machine"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, tc)
			got, err := f.lookup.MergeMachine(tc.key, tc.fallback)
			checkLookup(t, tc, got, err)
		})
	}
}

func TestMergeMachineCommand(t *testing.T) {
	t.Parallel()
	exe := "agentrepo"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	binary := filepath.Join(t.TempDir(), exe)
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../../cmd/agentrepo")
	if data, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, data)
	}
	f := newFixture(t, configCase{machine: "gh_login = machine\n"})
	gitFixture(t, f, "init", "--quiet")
	for _, tc := range []struct {
		key, fallback, want string
	}{
		{key: "gh_login", want: "machine\n"},
		{key: "unit_missing", fallback: "fallback", want: "fallback\n"},
		{key: "unit_missing", want: "\n"},
	} {
		got := runCommand(t, f.root, isolatedEnv(f), binary, "config", "merge-machine", tc.key, tc.fallback)
		if got != (result{out: tc.want}) {
			t.Fatalf("%s: %+v", tc.key, got)
		}
	}
	for _, key := range []string{"merge_owner", "required_reviewer", "required_reviewer_type", "delegation_check", "gh_login"} {
		for _, value := range []string{"override", ""} {
			name := "DEVTOOLS_" + strings.ToUpper(key)
			f.env[name] = value
			got := runCommand(t, f.root, isolatedEnv(f), binary, "config", "merge-machine", "gh_login")
			if got.code != 1 || got.out != "" || !strings.Contains(got.stderr, name) ||
				strings.Contains(got.stderr, "override") {
				t.Fatalf("%s: %+v", name, got)
			}
		}
		delete(f.env, "DEVTOOLS_"+strings.ToUpper(key))
	}
}
