// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package config_test

import (
	"path/filepath"
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, tc)
			got, err := f.lookup.MergeMachine(tc.key, tc.fallback)
			checkLookup(t, tc, got, err)
		})
	}
}

func TestRepositoryAt(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"golden_paths", "loosening_markers", "gate_config_paths", "owner_paths", "owner_review_label", "ruleset_dir", "merge_ready_cmd", "lint_squash_cmd"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, configCase{repository: key + " = pinned\n"})
			gitFixture(t, f, "init", "--quiet")
			gitFixture(t, f, "add", "--", ".agents/agent-skills.conf")
			gitFixture(
				t,
				f,
				"-c",
				"user.name=Tester",
				"-c",
				"user.email=tester@example.com",
				"commit",
				"--quiet",
				"-m",
				"test: pinned configuration",
			)
			pinned := runCommand(t, f.root, isolatedEnv(f), "git", "rev-parse", "HEAD").out
			for _, checkout := range []string{key + " = checkout\n", "unknown = invalid\n"} {
				writeFile(t, filepath.Join(f.root, ".agents", "agent-skills.conf"), checkout)
				got, differs, err := f.lookup.RepositoryAt(t.Context(), strings.TrimSpace(pinned), key, "fallback")
				if err != nil || got != "pinned" || !differs {
					t.Fatalf("%q, %v, %v", got, differs, err)
				}
			}
		})
	}
}

func TestRepositoryAtValidatesBase(t *testing.T) {
	t.Parallel()
	for _, tc := range []configCase{
		{name: "absent", fallback: "fallback", want: "fallback"},
		{name: "empty value", repository: "golden_paths =\n"},
		{name: "unknown", repository: "unknown = invalid\n", diagnostic: "Invalid or duplicate repository setting: unknown"},
		{name: "duplicate", repository: "golden_paths = a\ngolden_paths = b\n", diagnostic: "Invalid or duplicate repository setting: golden_paths"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, tc)
			gitFixture(t, f, "init", "--quiet")
			gitFixture(t, f, "add", "--", ".")
			gitFixture(
				t,
				f,
				"-c",
				"user.name=Tester",
				"-c",
				"user.email=tester@example.com",
				"commit",
				"--quiet",
				"-m",
				"test: pinned configuration",
			)
			got, _, err := f.lookup.RepositoryAt(t.Context(), "HEAD", "golden_paths", tc.fallback)
			checkLookup(t, tc, got, err)
			if _, _, err = f.lookup.RepositoryAt(t.Context(), "missing-ref", "golden_paths", ""); err == nil {
				t.Fatal("invalid base accepted")
			}
		})
	}
}
