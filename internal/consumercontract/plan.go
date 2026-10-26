// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrRefused reports a bump that must not start. The message gives the
// release the consumer has to adopt first.
var ErrRefused = errors.New("bump refused")

const (
	sourceDir = "source"
	targetDir = "target"
	planFile  = "plan.conf"
)

// Plan is what a bump to one target tag does to a consumer's conf.
type Plan struct {
	Target  *Document
	Actions []Entry
	// Conf is the consumer's conf as the bump leaves it.
	Conf string
	// Changed says whether Conf differs from the conf the consumer has.
	Changed bool
}

// Prepare decides, before anything is edited, whether the consumer at its
// current commit can move to the target tag, and what that move requires of
// its conf. state is a scratch directory the later steps read.
//
// The source reader is the consumer's own scripts at the checked-out commit,
// extracted into state. A neighboring release is never used as its evidence.
func Prepare(ctx context.Context, consumer, remote, tag, state string) (*Plan, error) {
	if err := os.MkdirAll(state, 0o750); err != nil {
		return nil, fmt.Errorf("create %s: %w", state, err)
	}
	repo, err := FetchTag(ctx, remote, tag, state)
	if err != nil {
		return nil, err
	}
	if err = Extract(ctx, repo, tag, filepath.Join(state, targetDir)); err != nil {
		return nil, fmt.Errorf("extract %s: %w", tag, err)
	}
	if err = Extract(ctx, consumer, "HEAD", filepath.Join(state, sourceDir)); err != nil {
		return nil, fmt.Errorf("extract the consumer's readers: %w", err)
	}
	conf, err := readConf(consumer)
	if err != nil {
		return nil, err
	}
	target, err := readContract(filepath.Join(state, targetDir))
	if err != nil {
		return nil, err
	}
	plan, err := newPlan(consumer, tag, conf, target)
	if err != nil {
		return nil, err
	}
	// A tag from before the contract declares nothing to apply, but both
	// readers still have to take the conf the bump leaves.
	if err = checkReaders(ctx, state, remote, tag, conf, plan); err != nil {
		return nil, err
	}
	if target == nil {
		return plan, nil
	}
	if err = os.WriteFile(filepath.Join(state, planFile), []byte(plan.Conf), 0o600); err != nil {
		return nil, fmt.Errorf("write plan: %w", err)
	}
	return plan, nil
}

// newPlan applies the target's entries to conf. A target without a contract
// declares nothing to apply.
func newPlan(consumer, tag, conf string, target *Document) (*Plan, error) {
	plan := &Plan{Target: target, Conf: conf}
	if target == nil {
		return plan, nil
	}
	if err := checkConsumerBaseline(consumer, target, tag); err != nil {
		return nil, err
	}
	for _, entry := range target.Transition.Entries {
		if applies(entry, plan.Conf, target) {
			plan.Actions = append(plan.Actions, entry)
			plan.Conf = edit(plan.Conf, entry, target)
		}
	}
	plan.Changed = plan.Conf != conf
	return plan, nil
}

// checkConsumerBaseline refuses a consumer that predates the contract
// unless the target is the baseline itself. The consumer's own tree says
// whether it predates the contract, because the contract file is part of
// every update after the baseline.
func checkConsumerBaseline(consumer string, target *Document, tag string) error {
	if target.Baseline {
		return nil
	}
	if _, err := os.Stat(filepath.Join(consumer, filepath.FromSlash(ContractPath))); err == nil {
		return nil
	}
	return fmt.Errorf(
		"%w: this repository predates the consumer contract, so it must first update to %s, the baseline, before it can update to %s",
		ErrRefused,
		target.BaselineTag,
		tag,
	)
}

// checkReaders refuses a bump when either reader rejects the planned conf,
// even if the conf was already invalid. It judges the plan before any edit,
// so no rejection appears after the pins have moved.
func checkReaders(ctx context.Context, state, remote, tag, before string, plan *Plan) error {
	source := filepath.Join(state, sourceDir)
	accepted, err := AcceptsConf(ctx, source, plan.Conf)
	if err != nil {
		return fmt.Errorf("source reader: %w", err)
	}
	if !accepted {
		reason := "the consumer's current reader would reject the result"
		if plan.Conf == before {
			reason = "the consumer's current reader rejects " + ConfFile + " as it stands"
		}
		return refuseSequence(plan, reason)
	}
	accepted, err = AcceptsConf(ctx, filepath.Join(state, targetDir), plan.Conf)
	if err != nil {
		return fmt.Errorf("target reader: %w", err)
	}
	if accepted {
		return nil
	}
	const reason = "the target reader would reject the result"
	if len(plan.Actions) == 0 {
		// The target declares nothing for this conf, so an earlier release
		// that did is the one to adopt first.
		guide, guideErr := bridgeFromHistory(ctx, state, remote, tag, plan.Conf)
		if guideErr != nil {
			return guideErr
		}
		if guide != "" {
			return fmt.Errorf("%w: %s, so update to %s first and bump again from there", ErrRefused, reason, guide)
		}
	}
	return refuseSequence(plan, reason)
}

func refuseSequence(plan *Plan, reason string) error {
	next := make([]string, 0, len(plan.Actions))
	for _, entry := range plan.Actions {
		next = append(next, entry.AfterRelease)
	}
	if len(next) == 0 {
		return fmt.Errorf("%w: %s", ErrRefused, reason)
	}
	return fmt.Errorf(
		"%w: %s, so update to %s first and bump again from there",
		ErrRefused,
		reason,
		strings.Join(next, " and "),
	)
}

// Apply writes the planned conf into the consumer's tree and returns
// whether it changed anything. It touches nothing when no action applies.
func Apply(consumer, state string) (bool, error) {
	planned, err := os.ReadFile(filepath.Join(state, planFile))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read plan: %w", err)
	}
	current, err := readConf(consumer)
	if err != nil {
		return false, err
	}
	if current == string(planned) {
		return false, nil
	}
	path := filepath.Join(consumer, filepath.FromSlash(ConfFile))
	//nolint:gosec // The consumer's own conf is written where the consumer's tree already keeps it.
	if err = os.WriteFile(path, planned, 0o600); err != nil {
		return false, fmt.Errorf("write %s: %w", ConfFile, err)
	}
	return true, nil
}

// Verify runs both readers over the consumer's tree as the bump left it:
// the source reader from the snapshot taken before any edit, and the target
// reader from the tag. A conf the source reader accepted before the bump
// must still pass both.
func Verify(ctx context.Context, consumer, state string) error {
	if _, err := os.Stat(filepath.Join(state, targetDir)); err != nil {
		return fmt.Errorf("%w: no prepared bump in %s", ErrRefused, state)
	}
	conf, err := readConf(consumer)
	if err != nil {
		return err
	}
	for _, reader := range []string{sourceDir, targetDir} {
		var accepted bool
		accepted, err = AcceptsConf(ctx, filepath.Join(state, reader), conf)
		if err != nil {
			return fmt.Errorf("%s reader: %w", reader, err)
		}
		if !accepted {
			return fmt.Errorf("%w: the %s reader rejects %s as the bump left it", ErrRefused, reader, ConfFile)
		}
	}
	return nil
}

func readConf(consumer string) (string, error) {
	data, err := os.ReadFile(filepath.Join(consumer, filepath.FromSlash(ConfFile)))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", ConfFile, err)
	}
	return string(data), nil
}

func readContract(root string) (*Document, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ContractPath)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil //nolint:nilnil // A tree without a contract is a valid answer, not a failure.
	}
	if err != nil {
		return nil, fmt.Errorf("read contract: %w", err)
	}
	return Parse(data)
}
