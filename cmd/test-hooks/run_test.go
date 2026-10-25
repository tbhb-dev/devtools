// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args       []string
		diagnostic string
	}{
		{"default path", nil, "discovering tests"},
		{"explicit path", []string{"missing"}, "discovering missing"},
		{"extra args", []string{"a", "b"}, "usage: test-hooks [path]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("HOOKS_JOBS", "1")
			var out, diagnostic bytes.Buffer
			if status := Run(
				tt.args,
				&out,
				&diagnostic,
			); status == 0 ||
				!strings.Contains(diagnostic.String(), tt.diagnostic) {
				t.Fatalf("status %d, diagnostic %q", status, diagnostic.String())
			}
		})
	}
}
