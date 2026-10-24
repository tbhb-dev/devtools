// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tbhb-dev/devtools/internal/consumercontract"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// TestBaselineContractMatchesTheShippedReader measures this repository's own
// readers, the shell one and the merge policy one, and requires the
// committed contract to equal what they do.
func TestBaselineContractMatchesTheShippedReader(t *testing.T) {
	t.Parallel()
	requireProbeTools(t)
	if _, err := exec.LookPath("uv"); err != nil {
		t.Skip("uv is not installed")
	}
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, consumercontract.ContractPath))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := consumercontract.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	probed, err := consumercontract.ProbeReader(t.Context(), root, doc.Keys)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range doc.Matrix() {
		if got := probed[id].Sorted(); !slices.Equal(got, want.Sorted()) {
			t.Errorf("%s: reader accepts %v, contract says %v", id, got, want.Sorted())
		}
	}
}

func TestProbeReaderMeasuresEachFixtureReader(t *testing.T) {
	t.Parallel()
	requireProbeTools(t)
	keys := parseFixture(t, "contract-require.toml").Keys
	tests := []struct {
		reader string
		key    string
		want   []string
	}{
		{"reader-v1", "alpha", []string{"absent", "empty", "nonempty"}},
		{"reader-v1", "gamma", []string{"absent"}},
		{"reader-v2", "gamma", []string{"absent", "empty", "nonempty"}},
		{"reader-v3", "gamma", []string{"empty", "nonempty"}},
	}
	for _, test := range tests {
		t.Run(test.reader+"/"+test.key, func(t *testing.T) {
			t.Parallel()
			matrix, err := consumercontract.ProbeReader(t.Context(), filepath.Join(fixtures, test.reader), keys)
			if err != nil {
				t.Fatal(err)
			}
			got := matrix[consumercontract.ConfFile+":"+test.key].Sorted()
			if !slices.Equal(got, test.want) {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestProbeReaderRejectsATreeWithoutAReader(t *testing.T) {
	t.Parallel()
	requireProbeTools(t)
	_, err := consumercontract.ProbeReader(t.Context(), t.TempDir(), parseFixture(t, "contract-baseline.toml").Keys)
	if err == nil {
		t.Fatal("a tree with no reader must not probe as a reader that rejects everything")
	}
}

func TestAcceptsConfJudgesAWholeConf(t *testing.T) {
	t.Parallel()
	requireProbeTools(t)
	v3 := filepath.Join(fixtures, "reader-v3")
	for conf, want := range map[string]bool{
		"gamma = on\n":              true,
		"alpha = 1\ngamma = on\n":   true,
		"alpha = 1\n":               false,
		"gamma = on\nunknown = 1\n": false,
		"# note\ngamma = on\n\n":    true,
	} {
		got, err := consumercontract.AcceptsConf(t.Context(), v3, conf)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("AcceptsConf(%q) = %v, want %v", conf, got, want)
		}
	}
}
