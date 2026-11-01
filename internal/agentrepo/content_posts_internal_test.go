// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestContentPostsDefault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "packages", "agents", "source", "nested")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(source, "post.sh")
	if err := os.WriteFile(path, []byte("gh pr comment 1 -b text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	code := 0
	cmd := contentPostsCommand(&code, root)
	cmd.SetArgs([]string{"scan"})
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if code != 1 || out.String() != path+":1\n" || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
}

func TestContentPostsExplicitReadFailure(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing.sh")
	var out, stderr bytes.Buffer
	code := Run([]string{"content-posts", "scan", path}, nil, &out, &stderr)
	if code != 2 || out.Len() != 0 || !bytes.Contains(stderr.Bytes(), []byte(path)) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
}
