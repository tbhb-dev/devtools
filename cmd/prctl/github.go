// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type githubCLI struct{}

func (githubCLI) output(ctx context.Context, name string, args []string, input string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = strings.NewReader(input)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	data, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return data, nil
}

type pullRequest struct {
	State      string `json:"state"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Comments struct {
		Nodes []struct {
			Body   string `json:"body"`
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
		} `json:"nodes"`
	} `json:"comments"`
}

type snapshot struct {
	login  string
	target pullRequest
}

func (s snapshot) hasMarker(marker string) bool {
	for _, comment := range s.target.Comments.Nodes {
		if comment.Author.Login == s.login && strings.HasSuffix(strings.TrimSpace(comment.Body), marker) {
			return true
		}
	}
	return false
}

func readSnapshot(ctx context.Context, client githubClient, repo string, request closeRequest) (snapshot, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return snapshot{}, errState
	}
	var query strings.Builder
	fmt.Fprintf(&query, `query { viewer { login } repository(owner: %q, name: %q) {
	nameWithOwner target: pullRequest(number: %s) {
	state repository { nameWithOwner } comments(last: 100) { nodes { body author { login } } }
	}`, owner, name, request.target)
	for i, number := range request.refs {
		fmt.Fprintf(&query, "ref%d: pullRequest(number: %s) { state repository { nameWithOwner } }\n", i, number)
	}
	query.WriteString("} }")
	data, err := client.output(ctx, "gh", []string{"query", "graphql", "-f", "query=" + query.String()}, "")
	if err != nil {
		return snapshot{}, err
	}
	return decodeSnapshot(data, repo, len(request.refs))
}

func decodeSnapshot(data []byte, repo string, count int) (snapshot, error) {
	var response struct {
		Data struct {
			Viewer struct {
				Login string `json:"login"`
			} `json:"viewer"`
			Repository map[string]json.RawMessage `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return snapshot{}, fmt.Errorf("read pull requests: %w", err)
	}
	result := snapshot{login: response.Data.Viewer.Login}
	if result.login == "" {
		return result, errState
	}
	for i := -1; i < count; i++ {
		key := fmt.Sprintf("ref%d", i)
		if i == -1 {
			key = "target"
		}
		var pr pullRequest
		if err := json.Unmarshal(response.Data.Repository[key], &pr); err != nil {
			return result, fmt.Errorf("read %s: %w", key, err)
		}
		if pr.Repository.NameWithOwner != repo || (pr.State != "OPEN" && (i == -1 || pr.State != "MERGED")) {
			return result, errState
		}
		if i == -1 {
			result.target = pr
		}
	}
	return result, nil
}
