// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/consumercontract"
)

// previousOf describes a preceding release whose reader behaves exactly as the
// named fixture contract says.
func previousOf(t *testing.T, tag, fixture string) consumercontract.Previous {
	t.Helper()
	doc := parseFixture(t, fixture)
	return consumercontract.Previous{Tag: tag, Contract: doc, Reader: doc.Matrix()}
}

func TestChangesNameEachKindOfRequirementChange(t *testing.T) {
	t.Parallel()
	optional := consumercontract.StateSet{
		consumercontract.StateAbsent:   true,
		consumercontract.StateEmpty:    true,
		consumercontract.StateNonempty: true,
	}
	required := consumercontract.StateSet{consumercontract.StateEmpty: true, consumercontract.StateNonempty: true}
	pinned := consumercontract.StateSet{consumercontract.ValueState("on"): true}
	file := consumercontract.ConfFile
	tests := map[string]struct {
		previous, candidate consumercontract.Matrix
		want                []string
	}{
		"required key added": {
			consumercontract.Matrix{file + ":k": optional},
			consumercontract.Matrix{file + ":k": required},
			[]string{"required-key-added " + file + ":k when key-absent"},
		},
		"unknown key becomes required": {
			consumercontract.Matrix{},
			consumercontract.Matrix{file + ":k": required},
			[]string{"required-key-added " + file + ":k when key-absent"},
		},
		"required value changed": {
			consumercontract.Matrix{file + ":k": required},
			consumercontract.Matrix{file + ":k": pinned},
			[]string{"required-value-changed " + file + ":k when value-rejected"},
		},
		"accepted value removed": {
			consumercontract.Matrix{file + ":k": optional},
			consumercontract.Matrix{},
			[]string{"accepted-value-removed " + file + ":k when value-rejected"},
		},
		"widened acceptance": {
			consumercontract.Matrix{file + ":k": required},
			consumercontract.Matrix{file + ":k": optional},
			nil,
		},
		"new optional key": {
			consumercontract.Matrix{},
			consumercontract.Matrix{file + ":k": optional},
			nil,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			changes := consumercontract.Changes(test.previous, test.candidate)
			got := make([]string, 0, len(changes))
			for _, change := range changes {
				got = append(got, change.ID())
			}
			if strings.Join(got, "|") != strings.Join(test.want, "|") {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

// AC3: an undeclared change to what the readers need fails and cites the entry.
func TestValidateReleaseFailsOnUndeclaredChange(t *testing.T) {
	t.Parallel()
	previous := previousOf(t, "v1.2.0", "contract-bridge.toml")
	doc := parseFixture(t, "contract-require-undeclared.toml")
	err := consumercontract.ValidateRelease(previous, doc)
	if !errors.Is(err, consumercontract.ErrUndeclared) {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "required-key-added "+consumercontract.ConfFile+":gamma") {
		t.Fatalf("error does not name the entry: %v", err)
	}
}

// AC3 and AC7: the change the bridge release prepared for is accepted.
func TestValidateReleaseAcceptsADeclaredTransitionAfterItsBridge(t *testing.T) {
	t.Parallel()
	previous := previousOf(t, "v1.2.0", "contract-bridge.toml")
	if err := consumercontract.ValidateRelease(previous, parseFixture(t, "contract-require.toml")); err != nil {
		t.Fatal(err)
	}
}

// AC7: the v0.16.0 shape. The preceding reader rejects the key, so no conf
// state exists that both readers accept, and a bridge release has to come first.
func TestValidateReleaseRejectsATransitionWithoutOverlap(t *testing.T) {
	t.Parallel()
	previous := previousOf(t, "v1.1.0", "contract-baseline.toml")
	err := consumercontract.ValidateRelease(previous, parseFixture(t, "contract-require-direct.toml"))
	if !errors.Is(err, consumercontract.ErrNoOverlap) || !strings.Contains(err.Error(), "bridge release") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateReleaseRejectsAWrongSequence(t *testing.T) {
	t.Parallel()
	previous := previousOf(t, "v1.2.0", "contract-bridge.toml")
	doc := parseFixture(t, "contract-require-direct.toml")
	err := consumercontract.ValidateRelease(previous, doc)
	if !errors.Is(err, consumercontract.ErrSequence) || !strings.Contains(err.Error(), "v1.2.0") {
		t.Fatalf("got %v", err)
	}
}

// AC8: an explicit none passes when nothing changed and fails when it did.
func TestValidateReleaseJudgesAnExplicitNone(t *testing.T) {
	t.Parallel()
	bridge := previousOf(t, "v1.2.0", "contract-bridge.toml")
	unchanged := parseFixture(t, "contract-bridge.toml")
	unchanged.BaselineTag = "v1.1.0"
	if err := consumercontract.ValidateRelease(bridge, unchanged); err != nil {
		t.Fatalf("unchanged contract with none: %v", err)
	}
	changed := parseFixture(t, "contract-require-undeclared.toml")
	if err := consumercontract.ValidateRelease(bridge, changed); err == nil {
		t.Fatal("a changed contract with none passed")
	}
}

func TestValidateReleaseRejectsADeclarationThatIsNotRequired(t *testing.T) {
	t.Parallel()
	previous := previousOf(t, "v1.2.0", "contract-bridge.toml")
	doc := parseFixture(t, "contract-require.toml")
	doc.Keys[2].Required = false
	if err := consumercontract.ValidateRelease(previous, doc); !errors.Is(err, consumercontract.ErrSpurious) {
		t.Fatalf("got %v", err)
	}
}

// AC9: the baseline matches the preceding reader or the release fails.
func TestValidateReleaseHoldsTheBaselineToThePrecedingReader(t *testing.T) {
	t.Parallel()
	baseline := parseFixture(t, "contract-baseline.toml")
	reader := baseline.Matrix()
	nothing := consumercontract.Previous{Tag: "v1.0.0", Reader: reader}
	if err := consumercontract.ValidateRelease(nothing, baseline); err != nil {
		t.Fatalf("matching baseline: %v", err)
	}
	changed := parseFixture(t, "contract-baseline.toml")
	changed.Keys[0].Required = true
	if err := consumercontract.ValidateRelease(nothing, changed); !errors.Is(err, consumercontract.ErrBaselineChanged) {
		t.Fatalf("baseline that tightens a key: %v", err)
	}
	widened := parseFixture(t, "contract-baseline.toml")
	widened.Keys = append(
		widened.Keys,
		consumercontract.Key{
			File:    consumercontract.ConfFile,
			Name:    "gamma",
			Accepts: []string{"empty", "nonempty"},
			Sample:  "x",
		},
	)
	if err := consumercontract.ValidateRelease(nothing, widened); !errors.Is(err, consumercontract.ErrBaselineChanged) {
		t.Fatalf("baseline that adds a key the reader rejects: %v", err)
	}
	later := consumercontract.Previous{Tag: "v1.1.0", Contract: baseline, Reader: reader}
	if err := consumercontract.ValidateRelease(later, baseline); !errors.Is(err, consumercontract.ErrBaseline) {
		t.Fatalf("second baseline: %v", err)
	}
}

func TestValidateReleaseNeedsAMatchingBaselineTag(t *testing.T) {
	t.Parallel()
	previous := previousOf(t, "v1.1.0", "contract-baseline.toml")
	doc := parseFixture(t, "contract-bridge.toml")
	doc.BaselineTag = "v0.9.0"
	if err := consumercontract.ValidateRelease(previous, doc); !errors.Is(err, consumercontract.ErrBaseline) {
		t.Fatalf("got %v", err)
	}
	if err := consumercontract.ValidateRelease(
		consumercontract.Previous{Tag: "v1.1.0", Reader: previous.Reader},
		doc,
	); !errors.Is(
		err,
		consumercontract.ErrBaseline,
	) {
		t.Fatalf("a first contract that is not a baseline: %v", err)
	}
}
