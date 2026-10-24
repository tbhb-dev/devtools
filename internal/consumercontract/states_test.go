// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract_test

import (
	"testing"

	"github.com/tbhb-dev/devtools/internal/consumercontract"
)

func TestStateSetCoversConcreteValues(t *testing.T) {
	t.Parallel()
	pinned := consumercontract.StateSet{}
	pinned.Add(consumercontract.ValueState("on"))
	anything := consumercontract.StateSet{consumercontract.StateEmpty: true, consumercontract.StateNonempty: true}
	for _, test := range []struct {
		set   consumercontract.StateSet
		value string
		want  bool
	}{
		{pinned, "on", true},
		{pinned, "off", false},
		{pinned, "", false},
		{anything, "", true},
		{anything, "whatever", true},
		{consumercontract.StateSet{consumercontract.StateAbsent: true}, "x", false},
	} {
		if got := test.set.Covers(test.value); got != test.want {
			t.Errorf("Covers(%q) on %v = %v", test.value, test.set.Sorted(), got)
		}
	}
	if both := pinned.Intersect(anything); len(both) != 0 {
		t.Errorf("intersection %v", both.Sorted())
	}
}
