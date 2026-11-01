// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

type issueState struct {
	State string `json:"state"`
	URL   string `json:"url"`
}

func (r *request) readIssueState(ctx context.Context) (issueState, error) {
	data, err := gh(ctx, []string{
		issueVerb, viewVerb, r.operand, repoFlag, r.repo, jsonFlag, "state,url",
	})
	var state issueState
	if err != nil || json.Unmarshal(data, &state) != nil ||
		!slices.Contains([]string{stateOpen, stateClosed}, state.State) ||
		!strings.EqualFold(state.URL, "https://github.com/"+r.repo+"/issues/"+r.operand) {
		return issueState{}, errEnvironment
	}
	return state, nil
}

func (r *request) checkIssueBodyTarget(ctx context.Context, errOut io.Writer) int {
	state, err := r.readIssueState(ctx)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot read issue state or target is not an issue")
		return exitEnvironment
	}
	if state.State == stateClosed && !r.allowClosed {
		fmt.Fprintln(errOut, "gh-post: issue is closed; use --allow-closed to replace its body")
		return 1
	}
	return 0
}

func (r *request) changeIssueState(ctx context.Context, body []byte, log, out, errOut io.Writer) int {
	state, err := r.readIssueState(ctx)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot read issue state")
		return exitEnvironment
	}
	wanted := stateOpen
	if r.operation == issueClose {
		wanted = stateClosed
	}
	if state.State == wanted {
		fmt.Fprintln(errOut, "gh-post: issue is already "+strings.ToLower(wanted))
		return 1
	}
	comment := request{
		operation: issueComment, repo: r.repo, operand: r.operand,
		private: r.private, secretsOnly: r.secretsOnly,
	}
	if code := postContent(ctx, comment, body, log, io.Discard, errOut); code != 0 {
		return code
	}
	args := []string{issueVerb, strings.TrimPrefix(r.operation, "issue "), r.operand, repoFlag, r.repo}
	if r.operation == issueClose {
		args = append(args, "--reason", r.reason)
	}
	if _, err = gh(ctx, args); err != nil {
		fmt.Fprintln(errOut, "gh-post: comment was posted; issue state write failed; check GitHub before retrying")
		return exitEnvironment
	}
	state, err = r.readIssueState(ctx)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot read issue back; do not retry or undo automatically")
		return exitEnvironment
	}
	if state.State != wanted {
		fmt.Fprintln(errOut, "gh-post: issue read-back contradicts requested state; do not retry or undo automatically")
		return 1
	}
	if err = appendAudit(log, r.operation, state.URL, body); err != nil {
		fmt.Fprintln(errOut, "gh-post: posted but audit append failed; do not retry")
		return exitEnvironment
	}
	fmt.Fprintln(out, state.URL)
	return 0
}
