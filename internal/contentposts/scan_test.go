// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package contentposts_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/contentposts"
)

func TestPythonCases(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/python_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string `json:"name"`
		Command  string `json:"command"`
		Detected bool   `json:"detected"`
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			path := writeScript(t, tc.Command)
			var out, stderr bytes.Buffer
			wantCode, wantOutput := 0, ""
			if tc.Detected {
				wantCode, wantOutput = 1, path+":1\n"
			}
			code := contentposts.Scan([]string{path}, &out, &stderr)
			if code != wantCode || out.String() != wantOutput || stderr.Len() != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
			}
		})
	}
}

func TestScanOutcomes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source, suffix string
		code                 int
	}{
		{"clean", "echo clean\n", "", 0},
		{"multiline", "gh pr \\\n comment 1 -b text\n", ":1\n", 1},
		{"environment", "GH_TOKEN=value gh issue create -b text\n", ":1\n", 1},
		{"same line", "gh issue create; gh pr comment 1\n", ":1\n", 1},
		{"parse error", "if then\n", "", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := writeScript(t, tc.source)
			var out, stderr bytes.Buffer
			code := contentposts.Scan([]string{path}, &out, &stderr)
			want := ""
			if tc.suffix != "" {
				want = path + tc.suffix
			}
			if code != tc.code || out.String() != want {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
			}
			if tc.code == 2 {
				assertParseError(t, stderr.String(), path)
			} else if stderr.Len() != 0 {
				t.Fatal(stderr.String())
			}
		})
	}
}

func assertParseError(t *testing.T, diagnostic, path string) {
	t.Helper()
	if !strings.Contains(diagnostic, path+": parse:") || !strings.Contains(diagnostic, "if") {
		t.Fatal(diagnostic)
	}
}

func TestMultipleFilesAndErrors(t *testing.T) {
	t.Parallel()
	first := writeScript(t, "# heading\ngh pr create\ncommand gh issue comment 1\n")
	second := writeScript(t, "env TOKEN=value gh api repos/o/r/pulls/1/reviews -XPOST\n")
	var out, stderr bytes.Buffer
	code := contentposts.Scan([]string{first, second}, &out, &stderr)
	want := first + ":2\n" + first + ":3\n" + second + ":1\n"
	if code != 1 || out.String() != want || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
	unreadable := t.TempDir() // A directory can't be read as a script, even as root.
	out.Reset()
	code = contentposts.Scan([]string{unreadable, second}, &out, &stderr)
	if code != 2 || out.String() != second+":1\n" || !strings.Contains(stderr.String(), unreadable+": read:") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
	out.Reset()
	stderr.Reset()
	code = contentposts.Scan([]string{unreadable}, &out, &stderr)
	if code != 2 || out.Len() != 0 || !strings.Contains(stderr.String(), unreadable) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
}

func TestSourceWideScan(t *testing.T) {
	t.Parallel()
	paths, err := contentposts.Scripts("../..")
	if err != nil || len(paths) == 0 {
		t.Fatalf("paths=%v err=%v", paths, err)
	}
	var out, stderr bytes.Buffer
	if code := contentposts.Scan(paths, &out, &stderr); code != 0 || out.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, &out, &stderr)
	}
}

func TestSourceBoundary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nested := filepath.Join(root, "packages/agents/source/skills/example")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nested, "post.sh")
	if err := os.WriteFile(path, []byte("gh pr comment 1 -b text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "ignored.txt"), []byte("gh pr create"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths, err := contentposts.Scripts(root)
	if err != nil || len(paths) != 1 || paths[0] != path {
		t.Fatalf("paths=%v err=%v", paths, err)
	}
	var out, stderr bytes.Buffer
	if code := contentposts.Scan(paths, &out, &stderr); code != 1 || out.String() != path+":1\n" || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
}

func writeScript(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "script.sh")
	data := []byte(source)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	before := sha256.Sum256(data)
	t.Cleanup(func() {
		after, err := os.ReadFile(path)
		if err != nil || sha256.Sum256(after) != before {
			t.Errorf("scanner modified %s: %v", path, err)
		}
	})
	return path
}
