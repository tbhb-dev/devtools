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
	"expected issue create|comment|edit|label|close|reopen, pr create|edit|comment|review|ready|close|label, comment edit, review edit, or release create|edit; use only supported flags and file input",
)

type request struct {
	operation   string
	repo        string
	bodyFile    string
	operand     string
	reviewID    string
	reason      string
	kind        string
	commit      string
	head        string
	commentFile string
	add         []string
	remove      []string
	flags       []string
	text        []string
	private     []string
	secretsOnly bool
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
	issueVerb      = "issue"
	issueComment   = "issue comment"
	issueCreate    = "issue create"
	issueClose     = "issue close"
	issueReopen    = "issue reopen"
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
		return setOnce(&r.repo, value)
	case bodyFileFlag:
		return setOnce(&r.bodyFile, value)
	case "--commit", headFlag:
		return r.addRevision(key, value)
	case "--comment-file":
		return setOnce(&r.commentFile, value)
	case "--reason":
		return setOnce(&r.reason, value)
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

func setOnce(field *string, value string) error {
	if *field != "" {
		return errUsage
	}
	*field = value
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
	case issueClose:
		values = append(values, "--reason")
	case issueReopen:
	case issueCreate:
		values = append(values, strings.Fields("--title --label --assignee --milestone")...)
	case "issue edit":
		values = append(
			values,
			strings.Fields("--title --add-label --remove-label --add-assignee --remove-assignee --milestone")...,
		)
		switches = []string{"--remove-milestone"}
	case issueComment, "pr comment":
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
	return flagSets(values, switches)
}

func flagSets(values, switches []string) (map[string]bool, map[string]bool, bool) {
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
	if r.operation == commentEdit && r.kind != issueVerb && r.kind != "review" {
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
	return r.validateIssueInput()
}

func (r *request) validateIssueInput() error {
	if r.isIssueStateChange() && r.bodyFile == "" ||
		r.operation == issueClose && r.reason != "completed" && r.reason != "not planned" {
		return errUsage
	}
	return nil
}

func (r *request) isIssueStateChange() bool {
	return r.operation == issueClose || r.operation == issueReopen
}
