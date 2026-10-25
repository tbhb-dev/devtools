// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package mergepolicy evaluates saved merge policy inputs without external effects.
package mergepolicy

import (
	"encoding/json"
	"errors"
	"fmt"
)

var (
	errAuthorType = errors.New("author type must be Bot or User when set")
	errHead       = errors.New("head must not be empty")
	errReviews    = errors.New("snapshot must contain a reviews array")
)

type review struct {
	Author struct {
		Login string `json:"login"`
		Type  string `json:"__typename"`
	} `json:"author"`
	State       string `json:"state"`
	SubmittedAt string `json:"submittedAt"`
	Commit      struct {
		OID string `json:"oid"`
	} `json:"commit"`
}

// Approved selects the latest decisive review from login, then checks its head
// and optional GraphQL author type. Equal timestamps favor the later snapshot
// entry, matching the shell predicate's stable sort followed by last.
func Approved(data []byte, login, head, authorType string) (bool, error) {
	if authorType != "" && authorType != "Bot" && authorType != "User" {
		return false, errAuthorType
	}
	if head == "" {
		return false, errHead
	}
	var snapshot struct {
		Reviews *[]review `json:"reviews"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return false, fmt.Errorf("read review snapshot: %w", err)
	}
	if snapshot.Reviews == nil {
		return false, errReviews
	}
	selected := latestDecisive(*snapshot.Reviews, login)
	return selected != nil && selected.State == "APPROVED" && selected.Commit.OID == head &&
		(authorType == "" || selected.Author.Type == authorType), nil
}

func latestDecisive(reviews []review, login string) *review {
	var selected *review
	for i := range reviews {
		candidate := &reviews[i]
		if candidate.Author.Login != login || login == "" {
			continue
		}
		switch candidate.State {
		case "APPROVED", "CHANGES_REQUESTED", "DISMISSED":
			if selected == nil || candidate.SubmittedAt >= selected.SubmittedAt {
				selected = candidate
			}
		}
	}
	return selected
}
