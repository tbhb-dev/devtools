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
	"expected issue create|comment|edit, pr create|edit|comment|review, comment edit, or release create|edit; use --body-file PATH or stdin and only supported flags",
)

type request struct {
	operation string
	repo      string
	bodyFile  string
	operand   string
	kind      string
	flags     []string
	text      []string
}

const (
	bodyFileFlag   = "--body-file"
	issueCreate    = "issue create"
	prCreate       = "pr create"
	commentEdit    = "comment edit"
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
	case "-R", "--repo":
		if r.repo != "" {
			return errUsage
		}
		r.repo = value
	case bodyFileFlag:
		if r.bodyFile != "" {
			return errUsage
		}
		r.bodyFile = value
	case "--kind":
		r.kind = value
	default:
		r.flags = append(r.flags, key, value)
		r.text = append(r.text, value)
	}
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
	case "pr review":
		switches = []string{"--approve", "--comment", "--request-changes"}
	case commentEdit:
		values = append(values, "--kind")
	case "release create":
		values = append(values, strings.Fields("--title --target")...)
		switches = []string{draftFlag, "--prerelease", "--verify-tag"}
	case "release edit":
		values = append(values, "--title")
		switches = []string{draftFlag, "--prerelease"}
	default:
		return nil, nil, false
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
	needsOperand := r.operation != issueCreate && r.operation != prCreate
	if needsOperand && r.operand == "" {
		return errUsage
	}
	if needsOperand && !strings.HasPrefix(r.operation, "release ") &&
		!regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(r.operand) {
		return errUsage
	}
	if r.operation == commentEdit && r.kind != "issue" && r.kind != "review" {
		return errUsage
	}
	if r.operation == "pr review" && len(r.flags) != 1 {
		return errUsage
	}
	if r.operation == issueCreate || r.operation == prCreate {
		if !slices.Contains(r.flags, "--title") {
			return errUsage
		}
	}
	return nil
}
