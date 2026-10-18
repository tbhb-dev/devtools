// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package commitlint_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/commitlint"
)

func TestConfig(t *testing.T) {
	t.Parallel()
	empty := func(string) string { return "" }
	root := t.TempDir()
	nested := filepath.Join(root, "sub")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	c, err := commitlint.LoadConfig(root, empty)
	if err != nil || !reflect.DeepEqual(c, commitlint.DefaultConfig()) {
		t.Fatalf("%#v, %v", c, err)
	}
	writeFile(t, filepath.Join(root, ".commit-lint.toml"), `types = ["custom"]
header_min_length = 1
header_max_length = 120
body_max_line_length = 80
footer_max_line_length = 150
require_signed_off_by = false
[identities]
`)
	c, err = commitlint.LoadConfig(nested, empty)
	want := commitlint.Config{[]string{"custom"}, 1, 120, 80, 150, false, commitlint.Identities{}}
	if err != nil || !reflect.DeepEqual(c, want) {
		t.Fatalf("%#v, %v", c, err)
	}
	env := map[string]string{
		"COMMITLINT_TYPES":                  "fix feat",
		"COMMITLINT_HEADER_MIN_LENGTH":      "2",
		"COMMITLINT_HEADER_MAX_LENGTH":      "90",
		"COMMITLINT_BODY_MAX_LINE_LENGTH":   "70",
		"COMMITLINT_FOOTER_MAX_LINE_LENGTH": "99",
		"COMMITLINT_REQUIRE_SIGNED_OFF_BY":  "1",
	}
	c, err = commitlint.LoadConfig(nested, func(key string) string { return env[key] })
	want = commitlint.Config{[]string{"fix", "feat"}, 2, 90, 70, 99, true, commitlint.Identities{}}
	if err != nil || !reflect.DeepEqual(c, want) {
		t.Fatalf("%#v, %v", c, err)
	}
	// A nested repository must not inherit the parent's config.
	writeFile(t, filepath.Join(nested, ".git"), "")
	c, err = commitlint.LoadConfig(nested, empty)
	if err != nil || !reflect.DeepEqual(c, commitlint.DefaultConfig()) {
		t.Fatalf("%#v, %v", c, err)
	}
}

func TestConfigErrors(t *testing.T) {
	t.Parallel()
	for _, content := range []string{"invalid = [", "types = []", "header_min_length = -1", "header_min_length = 100", "body_max_line_length = -1", "footer_max_line_length = -1", `[identities]
agents = [{name = "Agent"}]`, `[identities]
sign_off = [{email = ""}]`} {
		t.Run(content, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, ".commit-lint.toml"), content)
			if _, err := commitlint.LoadConfig(dir, func(string) string { return "" }); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".commit-lint.toml"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := commitlint.LoadConfig(dir, func(string) string { return "" }); err == nil {
		t.Fatal("expected read error")
	}
}

func TestConfigEnvironment(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"HEADER_MIN_LENGTH", "HEADER_MAX_LENGTH", "BODY_MAX_LINE_LENGTH", "FOOTER_MAX_LINE_LENGTH", "REQUIRE_SIGNED_OFF_BY"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			fullKey := "COMMITLINT_" + key
			_, err := commitlint.LoadConfig(t.TempDir(), func(k string) string {
				if k == fullKey {
					return "invalid"
				}
				return ""
			})
			if err == nil || !strings.Contains(err.Error(), fullKey) {
				t.Fatalf("%v", err)
			}
		})
	}
}

func TestConfigSignoffOverride(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"0", "2", "-1"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			c, err := commitlint.LoadConfig(t.TempDir(), func(key string) string {
				if key == "COMMITLINT_REQUIRE_SIGNED_OFF_BY" {
					return value
				}
				return ""
			})
			if err != nil || c.RequireSignedOffBy {
				t.Fatalf("%#v, %v", c, err)
			}
		})
	}
}
