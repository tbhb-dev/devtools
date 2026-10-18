// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestRun(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		args   []string
		input  string
		want   int
		output string
	}{
		{"version", []string{"version"}, "", 0, "agentrepo"},
		{"evaluate", []string{"checks", "evaluate"}, `{"statusCheckRollup":[]}`, 4, `"outcome":4`},
		{"required", []string{"checks", "evaluate", "--required", "lint"}, `{"statusCheckRollup":[{"__typename":"StatusContext","context":"ci","state":"SUCCESS"}]}`, 3, `"lint"`},
		{"head", []string{"checks", "evaluate", "--expected-head", "old"}, `{"headRefOid":"new","statusCheckRollup":[]}`, 6, `"outcome":6`},
		{"bad input", []string{"checks", "evaluate"}, `invalid`, 5, `"error"`},
		{"missing file", []string{"checks", "evaluate", "--file", "/nonexistent/checks.json"}, "", 5, `"error"`},
		{"bad flags", []string{"checks", "evaluate", "--unknown"}, "", 5, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, stderr bytes.Buffer
			got := agentrepo.Run(tt.args, strings.NewReader(tt.input), &out, &stderr)
			if got != tt.want || !strings.Contains(out.String(), tt.output) {
				t.Fatalf("code %d output %s stderr %s", got, &out, &stderr)
			}
		})
	}
}

func TestFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "checks.json")
	if err := os.WriteFile(path, []byte(`{"statusCheckRollup":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if got := agentrepo.Run([]string{"checks", "evaluate", "--file", path}, nil, &out, &out); got != 4 {
		t.Fatal(got)
	}
}

var errRead = errors.New("read failed")

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errRead }
func TestReadError(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if got := agentrepo.Run(
		[]string{"checks", "evaluate"},
		brokenReader{},
		&out,
		&out,
	); got != 5 ||
		!strings.Contains(out.String(), "read failed") {
		t.Fatal(got, out.String())
	}
}
