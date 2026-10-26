// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

//nolint:paralleltest // t.Chdir changes the process working directory.
func TestContentPostsDefault(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	source := filepath.Join("packages", "agents", "source", "nested")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(source, "post.sh")
	if err := os.WriteFile(path, []byte("gh pr comment 1 -b text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	code := agentrepo.Run([]string{"content-posts", "scan"}, nil, &out, &stderr)
	if code != 1 || out.String() != path+":1\n" || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
}

func TestContentPostsExplicitReadFailure(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing.sh")
	var out, stderr bytes.Buffer
	code := agentrepo.Run([]string{"content-posts", "scan", path}, nil, &out, &stderr)
	if code != 2 || out.Len() != 0 || !bytes.Contains(stderr.Bytes(), []byte(path)) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
}
