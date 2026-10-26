// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract

import "slices"

// Reserved state names. A pinned value is stored as `value=<text>`.
const (
	StateAbsent   = "absent"
	StateEmpty    = "empty"
	StateNonempty = "nonempty"
	valuePrefix   = "value="
)

// ValueState names the state of a key pinned to one exact value.
func ValueState(value string) string { return valuePrefix + value }

// StateSet is a set of key states.
type StateSet map[string]bool

// Add inserts a state.
func (s StateSet) Add(state string) { s[state] = true }

// Sorted lists the states in a stable order.
func (s StateSet) Sorted() []string {
	states := make([]string, 0, len(s))
	for state := range s {
		states = append(states, state)
	}
	slices.Sort(states)
	return states
}

// Intersect returns the states both sets hold.
func (s StateSet) Intersect(other StateSet) StateSet {
	both := StateSet{}
	for state := range s {
		if other[state] {
			both.Add(state)
		}
	}
	return both
}

// Covers says whether the set contains a concrete value. A concrete value
// matches its exact state, or the empty or nonempty class it belongs to.
func (s StateSet) Covers(value string) bool {
	if s[ValueState(value)] {
		return true
	}
	if value == "" {
		return s[StateEmpty]
	}
	return s[StateNonempty]
}
