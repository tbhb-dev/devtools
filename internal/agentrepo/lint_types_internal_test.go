// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLintTypesRelativeEntry(t *testing.T) {
	t.Parallel()
	join := filepath.Join
	list := func(entries ...string) string {
		return strings.Join(entries, string(filepath.ListSeparator))
	}
	dot := func(path string) string { return "." + string(filepath.Separator) + path }
	absolute := t.TempDir()
	binUV := join("bin", "uv")
	cases := []struct {
		name     string
		path     string
		pathEnv  string
		resolved string
		found    bool
	}{
		{"relative-entry", binUV, list("bin"), dot(binUV), true},
		{"dot-entry", "uv", list("."), dot("uv"), true},
		{"after-absolute-entry", binUV, list(absolute, "bin"), dot(binUV), true},
		{"implicit-current-directory", "uv", list(absolute), "", false},
		{"empty-entry-only", "uv", list(""), "", false},
		{"unmatched-relative-entry", binUV, list("other"), "", false},
		{"empty-path", "uv", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resolved, found := lintTypesRelativeEntry(tc.path, tc.pathEnv)
			require.Equal(t, tc.found, found)
			require.Equal(t, tc.resolved, resolved)
		})
	}
}
