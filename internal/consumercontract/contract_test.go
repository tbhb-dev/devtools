// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/consumercontract"
)

func fixtureText(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtures, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// AC2: a reader of the contract finds every field of a transition.
func TestParseExposesEveryTransitionField(t *testing.T) {
	t.Parallel()
	doc := parseFixture(t, "contract-require.toml")
	if doc.SchemaVersion != 1 || len(doc.Transition.Entries) != 1 {
		t.Fatalf("schema %d with %d entries", doc.SchemaVersion, len(doc.Transition.Entries))
	}
	entry := doc.Transition.Entries[0]
	want := consumercontract.Entry{
		Kind:         consumercontract.KindRequiredKeyAdded,
		File:         consumercontract.ConfFile,
		Key:          "gamma",
		AppliesWhen:  consumercontract.WhenKeyAbsent,
		TargetValue:  "on",
		AfterRelease: "v1.2.0",
		Verify:       "The reader accepts the conf and gamma is set.",
		Rollback:     "Delete the gamma line.",
	}
	if entry != want {
		t.Fatalf("got %+v, want %+v", entry, want)
	}
}

// AC6: a schema version the reader doesn't support is rejected outright.
func TestParseRejectsUnsupportedSchemaVersions(t *testing.T) {
	t.Parallel()
	base := fixtureText(t, "contract-baseline.toml")
	for _, version := range []string{"0", "2", "99"} {
		_, err := consumercontract.Parse(
			[]byte(strings.Replace(base, "schema_version = 1", "schema_version = "+version, 1)),
		)
		if !errors.Is(err, consumercontract.ErrUnsupportedSchema) {
			t.Errorf("schema %s: got %v", version, err)
		}
	}
	if _, err := consumercontract.Parse(
		[]byte(strings.Replace(base, "schema_version = 1\n", "", 1)),
	); !errors.Is(
		err,
		consumercontract.ErrUnsupportedSchema,
	) {
		t.Errorf("missing schema: got %v", err)
	}
	if _, err := consumercontract.Parse(
		[]byte("schema_version = ["),
	); !errors.Is(
		err,
		consumercontract.ErrInvalidContract,
	) {
		t.Errorf("malformed: got %v", err)
	}
}

func TestParseRejectsInvalidContracts(t *testing.T) {
	t.Parallel()
	baseline := fixtureText(t, "contract-baseline.toml")
	require := fixtureText(t, "contract-require.toml")
	tests := map[string]string{
		"unknown field":            baseline + "\nsurprise = 1\n",
		"no keys":                  "schema_version = 1\nbaseline = true\n[transition]\nnone = true\n",
		"no declaration":           strings.Replace(baseline, "[transition]\nnone = true\n", "", 1),
		"none and entries":         require + "\n[transition]\nnone = true\n",
		"baseline with transition": strings.Replace(require, "baseline_tag = \"v1.1.0\"", "baseline = true", 1),
		"baseline names a tag": strings.Replace(
			baseline,
			"baseline = true",
			"baseline = true\nbaseline_tag = \"v1.0.0\"",
			1,
		),
		"no baseline tag":  strings.Replace(require, "baseline_tag = \"v1.1.0\"\n", "", 1),
		"bad baseline tag": strings.Replace(require, "\"v1.1.0\"", "\"1.1\"", 1),
		"duplicate key":    baseline + "\n[[key]]\nfile = \".agents/agent-skills.conf\"\nname = \"alpha\"\naccepts = [\"empty\"]\nsample = \"x\"\n",
		"other file":       strings.Replace(baseline, "\".agents/agent-skills.conf\"", "\"other.conf\"", 1),
		"bad class":        strings.Replace(baseline, "\"empty\", \"nonempty\"", "\"anything\"", 1),
		"repeated class":   strings.Replace(baseline, "\"empty\", \"nonempty\"", "\"empty\", \"empty\"", 1),
		"no class":         strings.Replace(baseline, "accepts = [\"empty\", \"nonempty\"]", "accepts = []", 1),
		"no sample":        strings.Replace(baseline, "sample = \"x\"", "sample = \"\"", 1),
		"empty-only sample": strings.Replace(
			baseline,
			"accepts = [\"empty\", \"nonempty\"]\nsample = \"x\"",
			"accepts = [\"empty\"]\nsample = \"x\"",
			1,
		),
		"value without class": strings.Replace(baseline, "sample = \"x\"", "sample = \"x\"\nvalue = \"x\"", 1),
		"multi-line sample":   strings.Replace(baseline, "sample = \"x\"", "sample = \"a\\nb\"", 1),
		"no verification": strings.Replace(
			require,
			"verify = \"The reader accepts the conf and gamma is set.\"",
			"verify = \"\"",
			1,
		),
		"no rollback": strings.Replace(
			require,
			"rollback = \"Delete the gamma line.\"",
			"rollback = \"\"",
			1,
		),
		"unknown kind":      strings.Replace(require, "required-key-added", "renamed", 1),
		"unknown condition": strings.Replace(require, "key-absent", "sometimes", 1),
		"bad after_release": strings.Replace(
			require,
			"after_release = \"v1.2.0\"",
			"after_release = \"soon\"",
			1,
		),
		"unaccepted target": strings.Replace(
			require,
			"accepts = [\"empty\", \"nonempty\"]\nsample = \"x\"\n\n[[transition",
			"accepts = [\"empty\"]\nsample = \"x\"\n\n[[transition",
			1,
		),
		"unknown entry key": strings.Replace(require, "key = \"gamma\"", "key = \"\"", 1),
	}
	for name, text := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := consumercontract.Parse([]byte(text)); !errors.Is(err, consumercontract.ErrInvalidContract) {
				t.Fatalf("got %v, want an invalid contract", err)
			}
		})
	}
}

func TestParseAcceptsAnExactValueKey(t *testing.T) {
	t.Parallel()
	text := strings.Replace(
		fixtureText(t, "contract-baseline.toml"),
		"accepts = [\"empty\", \"nonempty\"]\nsample = \"x\"",
		"accepts = [\"value\"]\nvalue = \"on\"\nsample = \"on\"",
		1,
	)
	doc, err := consumercontract.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	key, _ := doc.Lookup(consumercontract.ConfFile, "alpha")
	if !key.AcceptsValue("on") || key.AcceptsValue("off") || key.AcceptsValue("") {
		t.Fatalf("exact value key accepts the wrong values: %+v", key)
	}
	if got := key.States().Sorted(); len(got) != 2 || got[1] != "value=on" {
		t.Fatalf("states %v", got)
	}
}
