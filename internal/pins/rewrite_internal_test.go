// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package pins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestUpdateRewriteSites(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	inputs := map[string]string{
		"apm.yml":                  "  - tbhb-dev/devtools#v0.10.0\n  - 'tbhb-dev/devtools/packages/agents/common#v0.10.0'\n  - tbhb-dev/devtools/packages/agents/agy#v0.10.0\n  - \"tbhb-dev/devtools/packages/agents/claude#v0.10.0\"\n  - tbhb-dev/devtools/packages/agents/codex#v0.10.0\n",
		"mise.toml":                "[tools]\n",
		"tombi.toml":               "# adapted from tbhb-dev/devtools v0.10.0.",
		".github/workflows/a.yml":  "uses: tbhb-dev/devtools/.github/workflows/a.yml@aaaaaaaa # v0.10.0\n",
		".github/workflows/b.yaml": "uses: tbhb-dev/devtools/.github/workflows/b.yml@aaaaaaaa # v0.10.0\n",
		".github/renovate.json5":   "\"github>tbhb-dev/devtools//.github/custom.json5#aaaaaaaa\", // v0.10.0\n\"github>tbhb-dev/devtools//.github/no-comment.json5#v0.10.0\"\n\"github>tbhb-dev/devtools//.github/unpinned.json5\"\n",
		"vendir.yml":               "url: https://github.com/tbhb-dev/devtools\n  ref: v0.10.0\nurl: https://example.com/other\n  ref: keep\nurl: https://github.com/tbhb-dev/devtools\n  ref: v0.10.0\n",
		".pre-commit-config.yaml":  "repo: https://github.com/tbhb-dev/devtools\n  rev: aaaaaaaa  # frozen: v0.10.0\n",
		"README.md":                "tbhb-dev/devtools v0.10.0\n",
	}
	for _, tool := range []string{"guard-markdown", "commit-lint", "gh-post", "agentrepo"} {
		inputs["mise.toml"] += `"go:github.com/tbhb-dev/devtools/cmd/` + tool + `" = "v0.10.0"` + "\n"
	}
	for name, body := range inputs {
		write(t, dir, name, body)
	}
	u := updater{dir: dir}
	if err := u.rewrite("v0.11.0", "bbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	for name, body := range inputs {
		want := strings.ReplaceAll(strings.ReplaceAll(body, "v0.10.0", "v0.11.0"), "aaaaaaaa", "bbbbbbbb")
		switch name {
		case "README.md":
			want = body
		case "tombi.toml":
			want += "\n"
		case ".github/renovate.json5":
			want = strings.ReplaceAll(want, "no-comment.json5#v0.11.0", "no-comment.json5#bbbbbbbb")
		}
		if got := read(t, dir, name); got != want {
			t.Errorf("%s:\ngot %q\nwant %q", name, got, want)
		}
	}
}
