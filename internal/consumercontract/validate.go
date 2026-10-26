// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

var (
	// ErrMissingContract reports a candidate tree without a contract.
	ErrMissingContract = errors.New("release candidate carries no consumer contract")
	// ErrNotesStale reports consumer notes that differ from the contract.
	ErrNotesStale = errors.New("consumer notes do not match the contract")
	// ErrContractDrift reports a contract that doesn't describe the
	// candidate's own reader.
	ErrContractDrift = errors.New("contract does not describe the candidate's reader")
)

// Candidate is the result of validating a release candidate tree.
type Candidate struct {
	Document    *Document
	PreviousTag string
}

// ValidateCandidate judges the exact tree in repo as the next release. It
// compares the candidate's contract against the reader of the newest tag
// reachable from the checked-out commit, whatever the declaration says. It
// modifies nothing in repo, so a release path can run it before its first
// mutation.
func ValidateCandidate(ctx context.Context, repo string) (*Candidate, error) {
	data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(ContractPath)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: add %s", ErrMissingContract, ContractPath)
	}
	if err != nil {
		return nil, fmt.Errorf("read contract: %w", err)
	}
	doc, err := Parse(data)
	if err != nil {
		return nil, err
	}
	if err = checkNotes(repo, doc); err != nil {
		return nil, err
	}
	tag, err := PreviousTag(ctx, repo, "HEAD")
	if err != nil {
		return nil, err
	}
	scratch, err := os.MkdirTemp("", "consumercontract-release-*")
	if err != nil {
		return nil, fmt.Errorf("create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)
	previous, err := loadPrevious(ctx, repo, tag, scratch)
	if err != nil {
		return nil, err
	}
	keys := unionKeys(doc.Keys, previousKeys(previous.Contract))
	previous.Reader, err = ProbeReader(ctx, filepath.Join(scratch, "previous"), keys)
	if err != nil {
		return nil, fmt.Errorf("measure %s: %w", tag, err)
	}
	if err = checkDrift(ctx, repo, doc); err != nil {
		return nil, err
	}
	if err = ValidateRelease(previous, doc); err != nil {
		return nil, err
	}
	return &Candidate{Document: doc, PreviousTag: tag}, nil
}

func checkNotes(repo string, doc *Document) error {
	notes, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(NotesPath)))
	if err != nil {
		return fmt.Errorf("%w: read %s: %w", ErrNotesStale, NotesPath, err)
	}
	if string(notes) != Render(doc) {
		return fmt.Errorf("%w: regenerate %s with `agentrepo contract render`", ErrNotesStale, NotesPath)
	}
	return nil
}

func loadPrevious(ctx context.Context, repo, tag, scratch string) (Previous, error) {
	previous := Previous{Tag: tag}
	if err := Extract(ctx, repo, tag, filepath.Join(scratch, "previous")); err != nil {
		return previous, fmt.Errorf("extract %s: %w", tag, err)
	}
	data, found, err := ReadAt(ctx, repo, tag, ContractPath)
	if err != nil || !found {
		return previous, err
	}
	previous.Contract, err = Parse(data)
	if err != nil {
		return previous, fmt.Errorf("%s contract: %w", tag, err)
	}
	return previous, nil
}

func previousKeys(doc *Document) []Key {
	if doc == nil {
		return nil
	}
	return doc.Keys
}

// unionKeys lists the first list's keys, then each key of the second whose
// ID and sample the first doesn't already carry. A key present in both lists
// with different samples appears twice. A probe then tries the exact values
// of both releases. The first list's samples come first.
func unionKeys(first, second []Key) []Key {
	keys := slices.Clone(first)
	for _, key := range second {
		if !slices.ContainsFunc(keys, func(k Key) bool { return k.ID() == key.ID() && k.Sample == key.Sample }) {
			keys = append(keys, key)
		}
	}
	return keys
}

// checkDrift holds the contract to the candidate's own reader: a contract
// that claims what its reader doesn't do would let any declaration pass.
func checkDrift(ctx context.Context, repo string, doc *Document) error {
	probed, err := ProbeReader(ctx, repo, doc.Keys)
	if err != nil {
		return fmt.Errorf("measure candidate: %w", err)
	}
	var problems []error
	for _, id := range sortedIDs(doc.Matrix()) {
		want, got := doc.Matrix()[id], probed[id]
		if !slices.Equal(want.Sorted(), got.Sorted()) {
			problems = append(
				problems,
				fmt.Errorf(
					"%w: %s says %v but the reader accepts %v",
					ErrContractDrift,
					id,
					want.Sorted(),
					got.Sorted(),
				),
			)
		}
	}
	return errors.Join(problems...)
}
