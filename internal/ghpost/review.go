// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func (r *request) matchesReviewBody(body []byte) bool {
	line, _, _ := strings.Cut(string(body), "\n")
	line = strings.TrimSuffix(line, "\r")
	head, ok := strings.CutPrefix(line, "Reviewed head: ")
	return ok && (strings.EqualFold(head, r.commit) || strings.EqualFold(head, "`"+r.commit+"`"))
}

func (r *request) checkReviewHead(ctx context.Context, errOut io.Writer) int {
	data, err := gh(ctx, []string{
		"pr", "view", r.operand, repoFlag, r.repo,
		"--json", "headRefOid", jqFlag, ".headRefOid",
	}, nil)
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
	return []string{apiCommand, "--method", "POST", endpoint, "--input", "-", jqFlag, ".html_url"}, input, nil
}
