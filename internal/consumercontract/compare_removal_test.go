// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract_test

import (
	"errors"
	"maps"
	"testing"

	"github.com/tbhb-dev/devtools/internal/consumercontract"
)

// AC7: removing a key the preceding reader requires has no overlap, while
// removing an optional key does.
func TestValidateReleaseJudgesTheOverlapOfARemovedKey(t *testing.T) {
	t.Parallel()
	removal := func(t *testing.T) *consumercontract.Document {
		t.Helper()
		doc := parseFixture(t, "contract-remove-required.toml")
		doc.BaselineTag = "v1.1.0"
		return doc
	}
	required := previousOf(t, "v1.3.0", "contract-require.toml")
	required.Contract.Baseline, required.Contract.BaselineTag = false, "v1.1.0"
	doc := removal(t)
	if err := consumercontract.ValidateRelease(required, doc); !errors.Is(err, consumercontract.ErrNoOverlap) {
		t.Fatalf("removing a required key: %v", err)
	}
	optional := previousOf(t, "v1.3.0", "contract-require.toml")
	optional.Contract.Baseline, optional.Contract.BaselineTag = false, "v1.1.0"
	optional.Reader = consumercontract.Matrix{}
	maps.Copy(optional.Reader, required.Reader)
	optional.Reader[consumercontract.ConfFile+":gamma"] = consumercontract.StateSet{
		consumercontract.StateAbsent:   true,
		consumercontract.StateEmpty:    true,
		consumercontract.StateNonempty: true,
	}
	if err := consumercontract.ValidateRelease(optional, removal(t)); err != nil {
		t.Fatalf("removing an optional key: %v", err)
	}
}
