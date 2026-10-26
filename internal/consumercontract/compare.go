// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var (
	// ErrUndeclared reports a required consumer change the release leaves out.
	ErrUndeclared = errors.New("undeclared consumer transition")
	// ErrSpurious reports a declared transition the readers don't require.
	ErrSpurious = errors.New("declared transition is not required")
	// ErrNoOverlap reports a transition with no state both readers accept.
	ErrNoOverlap = errors.New("no state is accepted by both the preceding and the candidate reader")
	// ErrBaseline reports a baseline that breaks the one-baseline rule.
	ErrBaseline = errors.New("consumer contract baseline rule broken")
	// ErrBaselineChanged reports a baseline whose contract differs from the
	// behavior of the reader in the preceding release.
	ErrBaselineChanged = errors.New("baseline contract differs from the preceding reader")
	// ErrSequence reports a transition sequenced against the wrong release.
	ErrSequence = errors.New("transition is sequenced against the wrong release")
)

// maxChangesPerKey bounds the changes one key can need: the key becoming
// required, and a value it used to take becoming rejected.
const maxChangesPerKey = 2

// Matrix maps a key ID (file:name) to the states a reader accepts.
type Matrix map[string]StateSet

// Matrix returns the states the contract says its readers accept.
func (d *Document) Matrix() Matrix {
	matrix := Matrix{}
	for _, key := range d.Keys {
		matrix[key.ID()] = key.States()
	}
	return matrix
}

// Change is one consumer change the move between two readers requires.
type Change struct {
	Kind        string
	File        string
	Key         string
	AppliesWhen string
}

// ID formats the change the way Entry.ID formats a declaration.
func (c Change) ID() string {
	return Entry{Kind: c.Kind, File: c.File, Key: c.Key}.ID() + " when " + c.AppliesWhen
}

// uncovered lists the states of previous that candidate rejects.
func uncovered(previous, candidate StateSet) []string {
	var lost []string
	for _, state := range previous.Sorted() {
		switch {
		case candidate[state]:
		case state == StateAbsent || state == StateEmpty:
			lost = append(lost, state)
		case state == StateNonempty:
			lost = append(lost, state)
		default:
			// The candidate still accepts a pinned value when it takes any nonempty value.
			if !candidate[StateNonempty] {
				lost = append(lost, state)
			}
		}
	}
	return lost
}

// Changes lists what a consumer in any state the previous reader accepts
// must change to be accepted by the candidate reader.
func Changes(previous, candidate Matrix) []Change {
	ids := make([]string, 0, len(candidate))
	for id := range candidate {
		ids = append(ids, id)
	}
	for id := range previous {
		if _, kept := candidate[id]; !kept {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	changes := make([]Change, 0, len(ids))
	for _, id := range ids {
		// A reader that doesn't list a key accepts only its absence, and
		// one that stops listing a key rejects it whenever the key is present.
		before, known := previous[id]
		if !known {
			before = StateSet{StateAbsent: true}
		}
		after, listed := candidate[id]
		if !listed {
			after = StateSet{StateAbsent: true}
		}
		changes = append(changes, changesFor(id, before, after)...)
	}
	return changes
}

// changesFor lists the changes one key's move from before to after needs.
func changesFor(id string, before, after StateSet) []Change {
	file, name, _ := strings.Cut(id, ":")
	changes := make([]Change, 0, maxChangesPerKey)
	var lostPresent bool
	for _, state := range uncovered(before, after) {
		if state == StateAbsent {
			changes = append(changes, Change{KindRequiredKeyAdded, file, name, WhenKeyAbsent})
			continue
		}
		lostPresent = true
	}
	if lostPresent {
		kind := KindRequiredValueChanged
		if !hasPresent(after) {
			kind = KindAcceptedRemoved
		}
		changes = append(changes, Change{kind, file, name, WhenValueRejected})
	}
	return changes
}

func hasPresent(states StateSet) bool {
	for state := range states {
		if state != StateAbsent {
			return true
		}
	}
	return false
}

// Previous describes the release before a candidate. Tag is its tag. Contract
// is its parsed contract, nil when the release has none. Reader is the
// behavior of its own reader, measured by running that reader.
type Previous struct {
	Tag      string
	Contract *Document
	Reader   Matrix
}

// ValidateRelease checks a release's declaration against the reader of the
// preceding release. It returns every problem rather than the first.
func ValidateRelease(previous Previous, doc *Document) error {
	changes := Changes(previous.Reader, doc.Matrix())
	var problems []error
	if err := checkBaseline(previous, doc); err != nil {
		problems = append(problems, err)
	}
	declared := make(map[string]Entry, len(doc.Transition.Entries))
	for _, entry := range doc.Transition.Entries {
		declared[Change{entry.Kind, entry.File, entry.Key, entry.AppliesWhen}.ID()] = entry
	}
	required := make(map[string]bool, len(changes))
	if doc.Baseline {
		problems = append(problems, baselineDiff(previous.Reader, doc.Matrix())...)
	}
	for _, change := range changes {
		required[change.ID()] = true
		if doc.Baseline {
			problems = append(problems, fmt.Errorf("%w: %s", ErrBaselineChanged, change.ID()))
			continue
		}
		entry, ok := declared[change.ID()]
		if !ok {
			problems = append(problems, fmt.Errorf("%w: %s", ErrUndeclared, change.ID()))
			continue
		}
		if err := checkSequence(entry, change, previous, doc); err != nil {
			problems = append(problems, err)
		}
	}
	for id := range declared {
		if !required[id] {
			problems = append(problems, fmt.Errorf("%w: %s", ErrSpurious, id))
		}
	}
	return errors.Join(problems...)
}

// checkBaseline keeps exactly one baseline in a history, and points every
// later release at it.
func checkBaseline(previous Previous, doc *Document) error {
	switch {
	case doc.Baseline && previous.Contract != nil:
		return fmt.Errorf(
			"%w: %s already carries a contract, so this release cannot be the baseline",
			ErrBaseline,
			previous.Tag,
		)
	case !doc.Baseline && previous.Contract == nil:
		return fmt.Errorf(
			"%w: %s carries no contract, so the first release with one must be a no-change baseline",
			ErrBaseline,
			previous.Tag,
		)
	case doc.Baseline:
		return nil
	}
	want := previous.Contract.BaselineTag
	if previous.Contract.Baseline {
		want = previous.Tag
	}
	if doc.BaselineTag != want {
		return fmt.Errorf("%w: baseline_tag is %s but the baseline release is %s", ErrBaseline, doc.BaselineTag, want)
	}
	return nil
}

// checkSequence requires the entry's target to be a state the preceding
// reader also accepts, and the entry to wait for exactly that reader.
func checkSequence(entry Entry, change Change, previous Previous, doc *Document) error {
	id := change.ID()
	before, known := previous.Reader[change.File+":"+change.Key]
	if !known {
		before = StateSet{StateAbsent: true}
	}
	key, kept := doc.Lookup(change.File, change.Key)
	switch {
	case change.AppliesWhen == WhenKeyAbsent && (!kept || !before.Covers(entry.TargetValue) || !key.AcceptsValue(entry.TargetValue)):
		return fmt.Errorf(
			"%w: %s (add a bridge release whose reader accepts the key before this one requires it)",
			ErrNoOverlap,
			id,
		)
	case change.AppliesWhen == WhenValueRejected && kept && !before.Covers(entry.TargetValue):
		return fmt.Errorf("%w: %s (target %q)", ErrNoOverlap, id, entry.TargetValue)
	case change.AppliesWhen == WhenValueRejected && !kept && !before[StateAbsent]:
		// A removed key's target state is its absence, and the preceding
		// reader must take that too, or the consumer has no safe step.
		return fmt.Errorf(
			"%w: %s (the preceding reader requires the key, so its absence is rejected)",
			ErrNoOverlap,
			id,
		)
	case entry.AfterRelease != previous.Tag:
		return fmt.Errorf(
			"%w: %s must wait for %s, the release whose reader accepts the target",
			ErrSequence,
			id,
			previous.Tag,
		)
	}
	return nil
}

// baselineDiff lists every key whose accepted states differ between the
// preceding reader and the baseline contract, in either direction.
func baselineDiff(reader, contract Matrix) []error {
	var diffs []error
	for _, id := range sortedIDs(contract) {
		want, got := contract[id], reader[id]
		if got == nil {
			got = StateSet{StateAbsent: true}
		}
		if !slices.Equal(want.Sorted(), got.Sorted()) {
			diffs = append(
				diffs,
				fmt.Errorf(
					"%w: %s accepts %v but the reader accepts %v",
					ErrBaselineChanged,
					id,
					want.Sorted(),
					got.Sorted(),
				),
			)
		}
	}
	return diffs
}

func sortedIDs(matrix Matrix) []string {
	ids := make([]string, 0, len(matrix))
	for id := range matrix {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
