// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package config_test

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/config"
)

type configCase struct {
	name, kind, key, fallback     string
	machine, defaults, repository string
	env                           map[string]string
	home                          bool
	want, diagnostic              string
}

// These fixtures are shared by the native lookup and old/new command tests.
func configCases() []configCase {
	return []configCase{
		{name: "default", kind: "machine", key: "review_effort", fallback: "fallback", want: "fallback"},
		{name: "implicit empty default", kind: "machine", key: "review_cmd"},
		{
			name:       "machine repository precedence",
			kind:       "machine",
			key:        "review_cmd",
			machine:    "# comment\nreview_cmd = from-repo-conf\n",
			defaults:   "review_cmd = defaults\n",
			repository: "review_cmd = planted\n",
			want:       "from-repo-conf",
		},
		{
			name:     "shared machine fallback",
			kind:     "machine",
			key:      "review_model",
			machine:  "review_cmd = other\n",
			defaults: "review_model = m1\n",
			want:     "m1",
		},
		{
			name:    "environment precedence",
			kind:    "machine",
			key:     "review_cmd",
			machine: "review_cmd = file\n",
			env:     map[string]string{"DEVTOOLS_REVIEW_CMD": "environment"},
			want:    "environment",
		},
		{
			name:     "empty environment wins",
			kind:     "machine",
			key:      "review_cmd",
			defaults: "review_cmd = defaults\n",
			env:      map[string]string{"DEVTOOLS_REVIEW_CMD": ""},
			fallback: "fallback",
		},
		{
			name: "environment whitespace",
			kind: "machine",
			key:  "review_cmd",
			env:  map[string]string{"DEVTOOLS_REVIEW_CMD": " value\n\n"},
			want: " value\n\n",
		},
		{
			name:     "empty machine value wins",
			kind:     "machine",
			key:      "review_cmd",
			machine:  "review_cmd = \t\n",
			defaults: "review_cmd = defaults\n",
			fallback: "fallback",
		},
		{
			name:    "first assignment wins",
			kind:    "machine",
			key:     "review_cmd",
			machine: "review_cmd = first\nreview_cmd = second\n",
			want:    "first",
		},
		{
			name:    "literal syntax",
			kind:    "machine",
			key:     "review_cmd",
			machine: " \t# comment\nignored\n\treview_cmd \t= \t'$(touch PWNED)' # literal = yes \t\n",
			want:    "'$(touch PWNED)' # literal = yes",
		},
		{name: "no final newline", kind: "machine", key: "review_cmd", machine: "review_cmd=value", want: "value"},
		{
			name:    "CR remains literal",
			kind:    "machine",
			key:     "review_cmd",
			machine: "review_cmd=value\r\n",
			want:    "value\r",
		},
		{
			name:     "HOME config fallback",
			kind:     "machine",
			key:      "review_cmd",
			defaults: "review_cmd = home\n",
			home:     true,
			want:     "home",
		},
		{
			name:       "repository cannot set machine authorities",
			kind:       "machine",
			key:        "gh_login",
			repository: "gh_login = planted\n",
			fallback:   "fallback",
			want:       "fallback",
		},
		{
			name:     "absent behavior file",
			kind:     "repository",
			key:      "required_hooks",
			fallback: "fallback",
			want:     "fallback",
		},
		{
			name:     "repository ignores environment and machine",
			kind:     "repository",
			key:      "required_hooks",
			machine:  "required_hooks=machine\n",
			env:      map[string]string{"DEVTOOLS_REQUIRED_HOOKS": "env"},
			fallback: "fallback",
			want:     "fallback",
		},
		{
			name:       "literal behavior",
			kind:       "repository",
			key:        "hook_marker",
			repository: " # comment\n\n hook_marker\t = $(touch PWNED) # literal\n",
			want:       "$(touch PWNED) # literal",
		},
		{
			name:       "empty behavior wins",
			kind:       "repository",
			key:        "lint_commit_cmd",
			repository: "lint_commit_cmd =\n",
			fallback:   "fallback",
		},
		{
			name:       "missing behavior key",
			kind:       "repository",
			key:        "required_hooks",
			repository: "hook_marker = marker\n",
			fallback:   "fallback",
			want:       "fallback",
		},
		{
			name:       "duplicate behavior key",
			kind:       "repository",
			key:        "review_max_rounds",
			repository: "review_max_rounds = 2\nreview_max_rounds = 3\n",
			diagnostic: "Invalid or duplicate repository setting: review_max_rounds",
		},
		{
			name:       "validate unrelated keys",
			kind:       "repository",
			key:        "required_hooks",
			repository: "required_hooks = ok\nunknown = secret\n",
			diagnostic: "Invalid or duplicate repository setting: unknown",
		},
		{
			name:       "reject missing equals",
			kind:       "repository",
			key:        "required_hooks",
			repository: "required_hooks\n",
			diagnostic: "Invalid or duplicate repository setting: required_hooks",
		},
		{
			name:       "report all invalid keys",
			kind:       "repository",
			key:        "required_hooks",
			repository: "bad = secret\nreview_cmd = private\n",
			diagnostic: "Invalid or duplicate repository setting: bad\nInvalid or duplicate repository setting: review_cmd",
		},
		{
			name:       "reject empty key",
			kind:       "repository",
			key:        "required_hooks",
			repository: " = value\n",
			diagnostic: "Invalid or duplicate repository setting: ",
		},
	}
}

type fixture struct {
	root   string
	env    map[string]string
	lookup config.Lookup
}

func newFixture(t *testing.T, tc configCase) fixture {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	configDir := filepath.Join(base, "config")
	home := filepath.Join(base, "home")
	env := map[string]string{"HOME": home, "XDG_CONFIG_HOME": configDir}
	if tc.home {
		env["XDG_CONFIG_HOME"] = ""
		configDir = filepath.Join(home, ".config")
	}
	maps.Copy(env, tc.env)
	for _, dir := range []string{filepath.Join(root, ".agents"), filepath.Join(configDir, "devtools")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for path, data := range map[string]string{
		filepath.Join(configDir, "devtools", "repo.conf"):     tc.machine,
		filepath.Join(configDir, "devtools", "devtools.conf"): tc.defaults,
		filepath.Join(root, ".agents", "agent-skills.conf"):   tc.repository,
		filepath.Join(root, "devtools.conf"):                  "review_cmd = planted\ngh_login = planted\n",
		filepath.Join(root, "agent-skills.conf"):              "review_cmd = planted\ngh_login = planted\n",
	} {
		if data != "" {
			writeFile(t, path, data)
		}
	}
	return fixture{root: root, env: env, lookup: config.Lookup{
		LookupEnv:      func(key string) (string, bool) { v, ok := env[key]; return v, ok },
		RepositoryName: func() (string, error) { return "repo", nil },
		RepositoryRoot: func() (string, error) { return root, nil },
	}}
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLookup(t *testing.T) {
	t.Parallel()
	for _, tc := range configCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, tc)
			read := f.lookup.Machine
			if tc.kind == "repository" {
				read = f.lookup.Repository
			}
			got, err := read(tc.key, tc.fallback)
			checkLookup(t, tc, got, err)
			if _, err = os.Stat(filepath.Join(f.root, "PWNED")); !os.IsNotExist(err) {
				t.Fatal("configuration was evaluated")
			}
		})
	}
}

func TestMachineAuthoritiesRejected(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"review_cmd", "review_model", "review_effort", "review_timeout", "delegation_check", "gh_login", "signoff_identity"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, configCase{repository: key + " = private\n"})
			if _, err := f.lookup.Repository("required_hooks", ""); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatal(err)
			}
		})
	}
}

func TestRepositoryKeys(t *testing.T) {
	t.Parallel()
	for key := range strings.FieldsSeq("lint_commit_cmd required_hooks hook_marker review_max_rounds pr_template lint_pr_cmd pr_log_entry pr_related_pattern pr_title_check_cmd owner_paths mail_cmd mail_to mail_from lint_squash_cmd owner_review_label ruleset_dir golden_paths constitutional_paths loosening_markers gate_config_paths disposable_ignored merge_ready_cmd") {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, configCase{repository: key + " = literal\n"})
			if got, err := f.lookup.Repository(key, ""); err != nil || got != "literal" {
				t.Fatalf("%q, %v", got, err)
			}
		})
	}
}

var errDiscovery = errors.New("git failed after printing a plausible path")

func TestDiscoveryFailures(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"machine", "repository"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, configCase{})
			fail := func() (string, error) { return f.root, errDiscovery }
			f.lookup.RepositoryName = fail
			f.lookup.RepositoryRoot = fail
			read := f.lookup.Machine
			if kind == "repository" {
				read = f.lookup.Repository
			}
			if got, err := read("review_cmd", "fallback"); !errors.Is(err, errDiscovery) || got != "" {
				t.Fatalf("%q, %v", got, err)
			}
		})
	}
}

func checkLookup(t *testing.T, tc configCase, got string, err error) {
	t.Helper()
	if tc.diagnostic != "" {
		if err == nil || err.Error() != tc.diagnostic || got != "" {
			t.Fatalf("got %q, %v; want refusal %q", got, err, tc.diagnostic)
		}
		return
	}
	if err != nil || got != tc.want {
		t.Fatalf("got %q, %v; want %q", got, err, tc.want)
	}
}

func TestEnvironmentSkipsDiscovery(t *testing.T) {
	t.Parallel()
	f := newFixture(t, configCase{env: map[string]string{"DEVTOOLS_REVIEW_CMD": ""}})
	f.lookup.RepositoryName = func() (string, error) { t.Fatal("unexpected Git discovery"); return "", errDiscovery }
	if got, err := f.lookup.Machine("review_cmd", "fallback"); err != nil || got != "" {
		t.Fatalf("%q, %v", got, err)
	}
}
