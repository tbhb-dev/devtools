// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// tagVersion splits a release tag into its three numbers.
func tagVersion(tag string) []int {
	var major, minor, patch int
	if _, err := fmt.Sscanf(tag, "v%d.%d.%d", &major, &minor, &patch); err != nil {
		return nil
	}
	return []int{major, minor, patch}
}

// earlierTags lists the release tags a remote publishes below tag, newest
// first.
func earlierTags(ctx context.Context, state, remote, tag string) ([]string, error) {
	out, err := git(ctx, state, hermeticEnv(state), "ls-remote", "--tags", "--refs", remote, "refs/tags/v*")
	if err != nil {
		return nil, err
	}
	var tags []string
	for line := range strings.SplitSeq(out, "\n") {
		_, ref, found := strings.Cut(line, "\trefs/tags/")
		if found && tagForm.MatchString(ref) && slices.Compare(tagVersion(ref), tagVersion(tag)) < 0 {
			tags = append(tags, ref)
		}
	}
	slices.SortFunc(tags, func(a, b string) int { return slices.Compare(tagVersion(b), tagVersion(a)) })
	return tags, nil
}

// bridgeFromHistory finds the release a conf has to adopt before the target,
// when the target itself declares nothing for it. Releases below the target
// are walked newest first, and the walk stops at the first whose declaration
// applies to the conf. That release is the answer if the consumer's reader
// already accepts the transition's result. Otherwise the answer is the
// bridge before it. The result is "" when no release below the target
// declares anything for the conf.
func bridgeFromHistory(ctx context.Context, state, remote, tag, conf string) (string, error) {
	tags, err := earlierTags(ctx, state, remote, tag)
	if err != nil {
		return "", err
	}
	for _, earlier := range tags {
		doc, readErr := publishedContract(ctx, state, remote, earlier)
		if readErr != nil || doc == nil {
			return "", readErr
		}
		trial, applied := applyDeclared(conf, doc)
		if applied.Kind == "" {
			continue
		}
		accepted, acceptErr := AcceptsConf(ctx, filepath.Join(state, sourceDir), trial)
		if acceptErr != nil {
			return "", fmt.Errorf("source reader: %w", acceptErr)
		}
		if accepted {
			return earlier, nil
		}
		return applied.AfterRelease, nil
	}
	return "", nil
}

// publishedContract reads the contract a published release contains. It
// returns nil for a release that has none.
func publishedContract(ctx context.Context, state, remote, tag string) (*Document, error) {
	repo, err := FetchTag(ctx, remote, tag, state)
	if err != nil {
		return nil, err
	}
	data, found, err := ReadAt(ctx, repo, tag, ContractPath)
	if err != nil || !found {
		return nil, err
	}
	doc, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s contract: %w", tag, err)
	}
	return doc, nil
}

// applyDeclared performs each entry of a declaration that applies to conf
// and returns the result with the last entry performed, empty when none was.
func applyDeclared(conf string, doc *Document) (string, Entry) {
	var applied Entry
	for _, entry := range doc.Transition.Entries {
		if applies(entry, conf, doc) {
			applied = entry
			conf = edit(conf, entry, doc)
		}
	}
	return conf, applied
}
