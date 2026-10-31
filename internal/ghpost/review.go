// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

func (r *request) matchesReviewBody(body []byte) bool {
	line, _, _ := strings.Cut(string(body), "\n")
	line = strings.TrimSuffix(line, "\r")
	head, ok := strings.CutPrefix(line, "Reviewed head: ")
	return ok && (strings.EqualFold(head, r.commit) || strings.EqualFold(head, "`"+r.commit+"`"))
}

var (
	errReviewFirstLine = errors.New("first line must be Reviewed head: <sha> matching --commit")
	errMissingRound    = errors.New("missing closing line Round: <n> of 3 (n from 1 to 3)")
	errUnclosedFence   = errors.New("unclosed fenced code block")
)

var bodyFence = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")

func (r *request) completeBody(body []byte) error {
	if r.operation == prReview && !r.matchesReviewBody(body) {
		return errReviewFirstLine
	}
	lines := strings.Split(string(body), "\n")
	first, last := "", ""
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) != "" {
			if first == "" {
				first = line
			}
			last = line
		}
	}
	refinement := r.operation == issueComment && strings.HasPrefix(first, "Refinement:")
	if !refinement && r.operation != prReview {
		return nil
	}
	if refinement && last != "Round: 1 of 3" && last != "Round: 2 of 3" && last != "Round: 3 of 3" {
		return errMissingRound
	}
	if unclosedBodyFence(lines) {
		return errUnclosedFence
	}
	return nil
}

func unclosedBodyFence(lines []string) bool {
	fence := ""
	for _, line := range lines {
		match := bodyFence.FindStringSubmatch(strings.TrimSuffix(line, "\r"))
		if match == nil {
			continue
		}
		if fence == "" {
			// A CommonMark backtick fence's info string can't contain backticks.
			if match[1][0] != '`' || !strings.Contains(match[2], "`") {
				fence = match[1]
			}
		} else if match[1][0] == fence[0] && len(match[1]) >= len(fence) && strings.Trim(match[2], " ") == "" {
			fence = ""
		}
	}
	return fence != ""
}

func (r *request) checkReviewHead(ctx context.Context, errOut io.Writer) int {
	data, err := gh(ctx, []string{
		"pr", viewVerb, r.operand, repoFlag, r.repo,
		jsonFlag, "headRefOid", jqFlag, ".headRefOid",
	})
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot read PR head")
		return exitEnvironment
	}
	if strings.TrimSpace(string(data)) != r.commit {
		fmt.Fprintln(errOut, "gh-post: PR head does not match --commit; review the current head again")
		return 1
	}
	return 0
}

func (r *request) reviewCommand(body []byte) ([]string, []byte, error) {
	event := map[string]string{
		"--approve": "APPROVE", "--comment": "COMMENT", "--request-changes": "REQUEST_CHANGES",
	}[r.flags[0]]
	input, err := json.Marshal(struct {
		CommitID string `json:"commit_id"`
		Event    string `json:"event"`
		Body     string `json:"body"`
	}{r.commit, event, string(body)})
	if err != nil {
		return nil, nil, fmt.Errorf("encode review: %w", err)
	}
	endpoint := "repos/" + r.repo + "/pulls/" + r.operand + "/reviews"
	return []string{apiCommand, methodFlag, "POST", endpoint, "--input", "-", jqFlag, ".html_url"}, input, nil
}
