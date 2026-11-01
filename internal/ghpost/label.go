// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// labelError is a refusal of the label arguments, with a message of its own.
type labelError string

func (e labelError) Error() string { return string(e) }

const (
	addFlag    = "--add"
	removeFlag = "--remove"

	labelPrefix = "change:"

	errLabelPrefix  labelError = "only labels starting with " + labelPrefix + " are accepted"
	errLabelValue   labelError = "a label must not contain a comma or a newline"
	errLabelOverlap labelError = "the same label cannot be both added and removed"
)

func (r *request) isLabelChange() bool {
	return r.operation == issueLabel || r.operation == prLabel
}

// labelFlags is the flag allowlist of the label operations, and refuses every other operation.
func labelFlags(operation string) (map[string]bool, map[string]bool, bool) {
	if operation != issueLabel && operation != prLabel {
		return nil, nil, false
	}
	values := map[string]bool{"-R": true, repoFlag: true, addFlag: true, removeFlag: true}
	return values, map[string]bool{}, true
}

// parseFailure preserves argument refusal messages.
func parseFailure(err error) error {
	if errors.Is(err, errIssueType) {
		return errIssueType
	}
	if refusal, ok := errors.AsType[labelError](err); ok {
		return refusal
	}
	return errUsage
}

func (r *request) addLabel(key, value string) {
	if key == addFlag {
		r.add = append(r.add, value)
	} else {
		r.remove = append(r.remove, value)
	}
	r.text = append(r.text, value)
}

// validateLabels settles every refusal that doesn't depend on GitHub's state.
func (r *request) validateLabels() error {
	if r.repo == "" || len(r.add)+len(r.remove) == 0 {
		return errUsage
	}
	for _, label := range slices.Concat(r.add, r.remove) {
		if strings.ContainsAny(label, ",\r\n") {
			return errLabelValue
		}
		if !strings.HasPrefix(strings.ToLower(label), labelPrefix) {
			return errLabelPrefix
		}
	}
	for _, label := range r.add {
		if hasLabel(r.remove, label) {
			return errLabelOverlap
		}
	}
	return nil
}

// hasLabel compares as GitHub does, without regard to case.
func hasLabel(labels []string, label string) bool {
	return slices.ContainsFunc(labels, func(other string) bool { return strings.EqualFold(other, label) })
}

type labelTarget struct {
	Pull   *bool    `json:"pull"`
	Labels []string `json:"labels"`
}

const labelTargetFilter = "{pull: (.pull_request != null), labels: [.labels[].name]}"

func (r *request) readLabelTarget(ctx context.Context) (labelTarget, int, string) {
	stdout, stderr, err := ghRun(ctx, []string{
		apiCommand, "repos/" + r.repo + "/issues/" + r.operand, jqFlag, labelTargetFilter,
	}, nil)
	if err != nil {
		if strings.Contains(stderr, "HTTP 404") {
			return labelTarget{}, 1, "target not found"
		}
		return labelTarget{}, exitEnvironment, "cannot read target"
	}
	var target labelTarget
	if json.Unmarshal(stdout, &target) != nil || target.Pull == nil {
		return labelTarget{}, exitEnvironment, "cannot read target"
	}
	if *target.Pull != (r.operation == prLabel) {
		if r.operation == prLabel {
			return labelTarget{}, 1, "target is an issue, not a pull request; use issue label"
		}
		return labelTarget{}, 1, "target is a pull request, not an issue; use pr label"
	}
	return target, 0, ""
}

// repositoryLabels lists the names the repository defines.
func (r *request) repositoryLabels(ctx context.Context) ([]string, error) {
	stdout, _, err := ghRun(
		ctx, []string{apiCommand, "repos/" + r.repo + "/labels", "--paginate", jqFlag, ".[].name"}, nil,
	)
	if err != nil {
		return nil, errEnvironment
	}
	return strings.Split(strings.TrimSpace(string(stdout)), "\n"), nil
}

// labelsToAdd keeps the labels the target lacks, spelled as the repository spells them.
// It refuses an unknown label here so no refusal can follow the first write.
func (r *request) labelsToAdd(ctx context.Context, target labelTarget, errOut io.Writer) ([]string, int) {
	var missing []string
	for _, label := range r.add {
		if !hasLabel(target.Labels, label) && !hasLabel(missing, label) {
			missing = append(missing, label)
		}
	}
	if len(missing) == 0 {
		return nil, 0
	}
	defined, err := r.repositoryLabels(ctx)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot read repository labels")
		return nil, exitEnvironment
	}
	add := make([]string, 0, len(missing))
	for _, label := range missing {
		index := slices.IndexFunc(defined, func(name string) bool { return strings.EqualFold(name, label) })
		if index < 0 {
			fmt.Fprintf(
				errOut,
				"gh-post: label %q does not exist in the repository; this command creates none\n",
				label,
			)
			return nil, 1
		}
		add = append(add, defined[index])
	}
	return add, 0
}

func (r *request) changeLabels(ctx context.Context, log, out, errOut io.Writer) int {
	target, code, message := r.readLabelTarget(ctx)
	if code != 0 {
		fmt.Fprintln(errOut, "gh-post:", message)
		return code
	}
	add, code := r.labelsToAdd(ctx, target, errOut)
	if code != 0 {
		return code
	}
	var remove []string
	for _, label := range r.remove {
		for _, name := range target.Labels {
			if strings.EqualFold(name, label) && !slices.Contains(remove, name) {
				remove = append(remove, name)
			}
		}
	}
	kind, _, _ := strings.Cut(r.operation, " ")
	path := "/issues/"
	if r.operation == prLabel {
		path = "/pull/"
	}
	url := "https://github.com/" + r.repo + path + r.operand
	if len(add)+len(remove) == 0 {
		fmt.Fprintln(out, "gh-post: labels already as requested, nothing changed:", url)
		return 0
	}
	args := []string{kind, "edit", r.operand, repoFlag, r.repo}
	for _, name := range add {
		args = append(args, "--add-label", name)
	}
	for _, name := range remove {
		args = append(args, "--remove-label", name)
	}
	if data, stderr, err := ghRun(ctx, args, nil); err != nil {
		fmt.Fprintln(errOut, "gh-post: gh write failed; check GitHub before retrying")
		r.printWriteError(stderr, nil, errOut)
		r.printWriteError(errorBody(data), nil, errOut)
		return exitEnvironment
	}
	if err := appendAudit(log, r.operation, url, nil); err != nil {
		fmt.Fprintln(errOut, "gh-post: posted but audit append failed; do not retry")
		return exitEnvironment
	}
	fmt.Fprintln(out, url)
	return 0
}
