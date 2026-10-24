// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"errors"
	"regexp"
	"slices"
	"strings"
)

var errUsage = errors.New(
	"expected issue create|comment|edit|label, pr create|edit|comment|review|ready|close|label, comment edit, review edit, or release create|edit; use only supported flags and file input",
)

type request struct {
	operation   string
	repo        string
	bodyFile    string
	operand     string
	reviewID    string
	kind        string
	commit      string
	head        string
	commentFile string
	add         []string
	remove      []string
	flags       []string
	text        []string
}

const (
	bodyFileFlag   = "--body-file"
	headFlag       = "--head"
	repoFlag       = "--repo"
	prReview       = "pr review"
	prReady        = "pr ready"
	prClose        = "pr close"
	issueLabel     = "issue label"
	prLabel        = "pr label"
	issueCreate    = "issue create"
	prCreate       = "pr create"
	commentEdit    = "comment edit"
	reviewEdit     = "review edit"
	draftFlag      = "--draft"
	operationWords = 2
)

func parse(args []string) (request, error) {
	r := request{}
	if len(args) < operationWords {
		return r, errUsage
	}
	r.operation = args[0] + " " + args[1]
	values, switches, ok := allowed(r.operation)
	if !ok {
		return r, errUsage
	}
	remaining := args[operationWords:]
	for len(remaining) > 0 {
		consumed, err := r.take(remaining, values, switches)
		if err != nil {
			return r, err
		}
		remaining = remaining[consumed:]
	}
	return r, r.validate()
}

func (r *request) take(args []string, values, switches map[string]bool) (int, error) {
	if !strings.HasPrefix(args[0], "-") {
		return 1, r.addOperand(args[0])
	}
	key, value, inline := strings.Cut(args[0], "=")
	if switches[key] && !inline {
		r.flags = append(r.flags, key)
		return 1, nil
	}
	if !values[key] {
		return 0, errUsage
	}
	if inline {
		return 1, r.addValue(key, value)
	}
	if len(args) < operationWords {
		return 0, errUsage
	}
	return operationWords, r.addValue(key, args[1])
}

func (r *request) addOperand(value string) error {
	if r.operation == reviewEdit && r.operand != "" && r.reviewID == "" {
		r.reviewID = value
		return nil
	}
	if r.operand != "" || r.operation == issueCreate || r.operation == prCreate {
		return errUsage
	}
	r.operand = value
	return nil
}

func (r *request) addValue(key, value string) error {
	if value == "" || strings.HasPrefix(value, "-") && (key != bodyFileFlag || value != "-") {
		return errUsage
	}
	switch key {
	case "-R", repoFlag:
		if r.repo != "" {
			return errUsage
		}
		r.repo = value
	case bodyFileFlag:
		if r.bodyFile != "" {
			return errUsage
		}
		r.bodyFile = value
	case "--commit", headFlag:
		return r.addRevision(key, value)
	case "--comment-file":
		if r.commentFile != "" {
			return errUsage
		}
		r.commentFile = value
	case "--kind":
		r.kind = value
	case addFlag, removeFlag:
		r.addLabel(key, value)
	default:
		r.flags = append(r.flags, key, value)
		r.text = append(r.text, value)
	}
	return nil
}

func (r *request) addRevision(key, value string) error {
	if key == headFlag && !r.isPRStateChange() {
		r.flags = append(r.flags, key, value)
		r.text = append(r.text, value)
		return nil
	}
	target := &r.commit
	if key == headFlag {
		target = &r.head
	}
	if *target != "" || !regexp.MustCompile(`^[0-9a-fA-F]{40}$`).MatchString(value) {
		return errUsage
	}
	*target = strings.ToLower(value)
	return nil
}

func allowed(operation string) (map[string]bool, map[string]bool, bool) {
	values := strings.Fields("-R --repo --body-file")
	var switches []string
	switch operation {
	case issueCreate:
		values = append(values, strings.Fields("--title --label --assignee --milestone")...)
	case "issue edit":
		values = append(
			values,
			strings.Fields("--title --add-label --remove-label --add-assignee --remove-assignee --milestone")...,
		)
		switches = []string{"--remove-milestone"}
	case "issue comment", "pr comment":
		switches = []string{"--edit-last", "--create-if-none"}
	case prCreate:
		values = append(values, strings.Fields("--title --base --head --label --assignee --reviewer --milestone")...)
		switches = []string{draftFlag}
	case "pr edit":
		values = append(
			values,
			strings.Fields(
				"--title --base --add-label --remove-label --add-assignee --remove-assignee --add-reviewer --remove-reviewer --milestone",
			)...,
		)
		switches = []string{"--remove-milestone"}
	case prReview:
		values = append(values, "--commit")
		switches = []string{"--approve", "--comment", "--request-changes"}
	case prReady, prClose:
		values = strings.Fields(map[string]string{
			prReady: "-R --repo --head",
			prClose: "-R --repo --head --comment-file",
		}[operation])
		switches = []string{"--dry-run"}
	case commentEdit:
		values = append(values, "--kind")
	case reviewEdit:
	case "release create":
		values = append(values, strings.Fields("--title --target")...)
		switches = []string{draftFlag, "--prerelease", "--verify-tag"}
	case "release edit":
		values = append(values, "--title")
		switches = []string{draftFlag, "--prerelease"}
	default:
		return labelFlags(operation)
	}
	v, s := map[string]bool{}, map[string]bool{}
	for _, k := range values {
		v[k] = true
	}
	for _, k := range switches {
		s[k] = true
	}
	return v, s, true
}

func (r *request) validate() error {
	if err := r.validateOperands(); err != nil {
		return err
	}
	if r.operation == commentEdit && r.kind != "issue" && r.kind != "review" {
		return errUsage
	}
	if r.operation == prReview && (len(r.flags) != 1 || r.commit == "") {
		return errUsage
	}
	if r.isPRStateChange() && r.head == "" || r.operation == prClose && r.commentFile == "" {
		return errUsage
	}
	if r.operation == issueCreate || r.operation == prCreate {
		if !slices.Contains(r.flags, "--title") {
			return errUsage
		}
	}
	return nil
}

func (r *request) isPRStateChange() bool {
	return r.operation == prReady || r.operation == prClose
}

func (r *request) validateOperands() error {
	needsOperand := r.operation != issueCreate && r.operation != prCreate
	if needsOperand && r.operand == "" {
		return errUsage
	}
	if needsOperand && !strings.HasPrefix(r.operation, "release ") &&
		!regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(r.operand) {
		return errUsage
	}
	if r.operation == reviewEdit && !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(r.reviewID) {
		return errUsage
	}
	if r.isLabelChange() {
		return r.validateLabels()
	}
	return nil
}
