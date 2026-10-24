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
	"unicode/utf8"
)

const (
	maxCommentCharacters = 65536
	stateOpen            = "OPEN"
)

var (
	errReadBody    = errors.New("cannot read body")
	errEmptyReason = errors.New("close reason must not be empty")
	errReasonSize  = errors.New("close reason exceeds comment size limit")
)

func (r *request) readInput(in io.Reader) ([]byte, error) {
	if r.operation == prReady {
		return nil, nil
	}
	path := r.bodyFile
	if r.operation == prClose {
		path = r.commentFile
	}
	body, err := readBody(path, in)
	if err != nil {
		return nil, errReadBody
	}
	if r.operation == prClose {
		if strings.TrimSpace(string(body)) == "" {
			return nil, errEmptyReason
		}
		if utf8.RuneCount(body) > maxCommentCharacters {
			return nil, errReasonSize
		}
	}
	return body, nil
}

type prState struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	IsDraft *bool  `json:"isDraft"`
	Head    string `json:"headRefOid"`
	URL     string `json:"url"`
}

func (r *request) readPRState(ctx context.Context) (prState, error) {
	data, err := gh(ctx, []string{
		"pr", viewVerb, r.operand, repoFlag, r.repo, jsonFlag, "id,state,isDraft,headRefOid,url",
	}, nil)
	var state prState
	if err != nil || json.Unmarshal(data, &state) != nil || state.ID == "" || state.IsDraft == nil ||
		state.Head == "" || !strings.EqualFold(state.URL, "https://github.com/"+r.repo+"/pull/"+r.operand) ||
		!slices.Contains([]string{stateOpen, "CLOSED", "MERGED"}, state.State) {
		return prState{}, errEnvironment
	}
	return state, nil
}

func (r *request) changePRState(ctx context.Context, body []byte, login string, log, out, errOut io.Writer) int {
	state, err := r.readPRState(ctx)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot read PR state")
		return exitEnvironment
	}
	if code := r.checkPRState(state, errOut); code != 0 {
		return code
	}
	if slices.Contains(r.flags, "--dry-run") {
		reason := ""
		if r.operation == prClose {
			reason = " with a reason comment if not already posted by this identity"
		}
		fmt.Fprintf(out, "gh-post: would %s %s%s\n", strings.TrimPrefix(r.operation, "pr "), state.URL, reason)
		return 0
	}
	if r.operation == prClose {
		if code := r.postCloseReason(ctx, body, login, log, errOut); code != 0 {
			return code
		}
	}
	args := []string{"pr", "close", r.operand, repoFlag, r.repo}
	if r.operation == prReady {
		args = []string{
			apiCommand, "graphql", "-f",
			"query=mutation($id:ID!){markPullRequestReadyForReview(input:{pullRequestId:$id}){pullRequest{url}}}",
			"-f", "id=" + state.ID,
		}
	}
	_, writeErr := gh(ctx, args, nil)
	return r.readBackPR(ctx, body, writeErr, log, out, errOut)
}

func (r *request) checkPRState(state prState, errOut io.Writer) int {
	var message string
	switch {
	case !strings.EqualFold(state.Head, r.head):
		message = "PR head does not match --head"
	case state.State != stateOpen:
		message = "PR is not open"
	case r.operation == prReady && !*state.IsDraft:
		message = "PR is not a draft"
	default:
		return 0
	}
	fmt.Fprintln(errOut, "gh-post:", message)
	return 1
}

func (r *request) postCloseReason(ctx context.Context, body []byte, login string, log, errOut io.Writer) int {
	data, err := gh(ctx, []string{
		apiCommand,
		methodFlag,
		"GET",
		"repos/" + r.repo + "/issues/" + r.operand + "/comments",
		"--paginate",
		"--slurp",
	}, nil)
	var pages [][]struct {
		Body string `json:"body"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err != nil || json.Unmarshal(data, &pages) != nil {
		fmt.Fprintln(errOut, "gh-post: cannot read PR comments")
		return exitEnvironment
	}
	for _, page := range pages {
		for _, comment := range page {
			if comment.User.Login == login && comment.Body == string(body) {
				return 0
			}
		}
	}
	comment := request{operation: "pr comment", repo: r.repo, operand: r.operand}
	return postContent(ctx, comment, body, log, io.Discard, errOut)
}

func (r *request) readBackPR(ctx context.Context, body []byte, writeErr error, log, out, errOut io.Writer) int {
	state, err := r.readPRState(ctx)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot read PR back; do not retry or undo automatically")
		return exitEnvironment
	}
	if !strings.EqualFold(state.Head, r.head) {
		fmt.Fprintln(errOut, "gh-post: PR read-back head does not match --head; do not retry or undo automatically")
		return 1
	}
	if writeErr != nil {
		if r.operation == prClose && state.State == stateOpen {
			fmt.Fprintln(
				errOut,
				"gh-post: reason comment was posted; close failed and PR is still open; check GitHub before retrying",
			)
		} else {
			fmt.Fprintln(errOut, "gh-post: PR state write failed; check GitHub before retrying")
		}
		return exitEnvironment
	}
	if r.operation == prClose && state.State != "CLOSED" ||
		r.operation == prReady && (state.State != stateOpen || *state.IsDraft) {
		fmt.Fprintln(errOut, "gh-post: PR read-back contradicts requested state; do not retry or undo automatically")
		return 1
	}
	if err = appendAudit(log, r.operation, state.URL, body); err != nil {
		fmt.Fprintln(errOut, "gh-post: posted but audit append failed; do not retry")
		return exitEnvironment
	}
	fmt.Fprintln(out, state.URL)
	return 0
}
