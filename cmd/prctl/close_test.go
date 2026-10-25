// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var errFake = errors.New("fake command failure")

type fakeGitHub struct {
	calls       []string
	body        string
	targetState string
	refState    string
	refRepo     string
	missing     bool
	marker      string
	author      string
	failLint    bool
	leak        bool
	failPost    bool
	failClose   bool
	posts       int
	closes      int
}

func (f *fakeGitHub) output(_ context.Context, name string, args []string, input string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	switch {
	case strings.HasPrefix(call, "gh repo view"):
		return []byte("owner/repo"), nil
	case strings.HasPrefix(call, "gh query graphql"):
		return f.snapshot()
	case strings.HasPrefix(call, "mise run lint-draft "):
		if f.failLint {
			return nil, errFake
		}
	case strings.HasPrefix(call, "gh-post pr comment "):
		f.body = input
		if f.leak && strings.Contains(input, "private-value") {
			return nil, errFake
		}
		f.posts++
		f.marker, f.author = input, "agent"
		if f.failPost {
			return nil, errFake
		}
	case strings.HasPrefix(call, "gh pr close "):
		if f.failClose {
			return nil, errFake
		}
		f.closes++
	default:
		return nil, errFake
	}
	return nil, nil
}

func (f *fakeGitHub) snapshot() ([]byte, error) {
	target := map[string]any{
		"state":      "OPEN",
		"repository": map[string]string{"nameWithOwner": "owner/repo"},
		"comments":   map[string]any{"nodes": []any{}},
	}
	if f.targetState != "" {
		target["state"] = f.targetState
	}
	if f.marker != "" {
		target["comments"] = map[string]any{
			"nodes": []any{map[string]any{"body": f.marker, "author": map[string]string{"login": f.author}}},
		}
	}
	state, repo := "OPEN", "owner/repo"
	if f.refState != "" {
		state = f.refState
	}
	if f.refRepo != "" {
		repo = f.refRepo
	}
	ref := any(map[string]any{"state": state, "repository": map[string]string{"nameWithOwner": repo}})
	if f.missing {
		ref = nil
	}
	data, err := json.Marshal(
		map[string]any{
			"data": map[string]any{
				"viewer":     map[string]string{"login": "agent"},
				"repository": map[string]any{"nameWithOwner": "owner/repo", "target": target, "ref0": ref, "ref1": ref},
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	return data, nil
}

func TestAC1GrantedCloseOnly(t *testing.T) {
	t.Parallel()
	f := &fakeGitHub{}
	require.Error(t, run(context.Background(), []string{"close", "1", "--duplicate-of", "2"}, f, false))
	require.Empty(t, f.calls)
	require.NoError(t, run(context.Background(), []string{"close", "1", "--duplicate-of", "2"}, f, true))
	require.Len(t, f.calls, 4)
	require.Equal(t, 1, f.posts)
	require.Equal(t, 1, f.closes)
}

func TestAC2RefusesInvalidState(t *testing.T) {
	t.Parallel()
	for name, f := range map[string]*fakeGitHub{
		"closed target":     {targetState: "CLOSED"},
		"merged target":     {targetState: "MERGED"},
		"missing reference": {missing: true},
		"closed reference":  {refState: "CLOSED"},
		"other repository":  {refRepo: "other/repo"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Error(t, run(context.Background(), []string{"close", "1", "--superseded-by", "2"}, f, true))
			require.Zero(t, f.posts)
			require.Zero(t, f.closes)
		})
	}
}

func TestAC3Comment(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args []string
		body string
	}{
		{[]string{"--superseded-by", "2", "3"}, "Superseded by owner/repo#2, owner/repo#3.\n\n<!-- closes-as: superseded -->"},
		{[]string{"--duplicate-of", "2"}, "Duplicate of owner/repo#2.\n\n<!-- closes-as: duplicate -->"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			t.Parallel()
			f := &fakeGitHub{refState: "MERGED"}
			require.NoError(t, run(context.Background(), append([]string{"close", "1"}, tc.args...), f, true))
			require.Equal(t, tc.body, f.body)
			require.Contains(t, f.calls[2], "gh-post pr comment 1 --repo owner/repo --body-file -")
		})
	}
}

func TestAC3ReasonGates(t *testing.T) {
	t.Parallel()
	for name, f := range map[string]*fakeGitHub{"lint": {failLint: true}, "leak": {leak: true}, "clean": {}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			reason := filepath.Join(t.TempDir(), "reason.md")
			text := "Replacement covers the remaining work."
			if f.leak {
				text = "private-value"
			}
			require.NoError(t, os.WriteFile(reason, []byte(text), 0o600))
			err := run(context.Background(), []string{"close", "1", "--duplicate-of", "2", "--reason", reason}, f, true)
			require.Contains(t, strings.Join(f.calls, "\n"), "mise run lint-draft "+reason)
			if name == "clean" {
				require.NoError(t, err)
				require.Contains(t, f.body, "Replacement covers the remaining work.\n\n<!-- closes-as: duplicate -->")
			} else {
				require.Error(t, err)
				require.Zero(t, f.posts)
				require.Zero(t, f.closes)
			}
		})
	}
}

func TestAC4PreservesBranch(t *testing.T) {
	t.Parallel()
	f := &fakeGitHub{}
	require.NoError(t, run(context.Background(), []string{"close", "1", "--duplicate-of", "2"}, f, true))
	require.Equal(t, "gh pr close 1 --repo owner/repo", f.calls[len(f.calls)-1])
}

func TestAC7Retry(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"close", "post audit"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			f := &fakeGitHub{failClose: failure == "close", failPost: failure == "post audit"}
			args := []string{"close", "1", "--duplicate-of", "2"}
			require.Error(t, run(context.Background(), args, f, true))
			require.Equal(t, 1, f.posts)
			require.Zero(t, f.closes)
			f.failClose, f.failPost = false, false
			require.NoError(t, run(context.Background(), args, f, true))
			require.Equal(t, 1, f.posts)
			require.Equal(t, 1, f.closes)
		})
	}
}

func TestAC7MarkerRequiresActiveAuthor(t *testing.T) {
	t.Parallel()
	f := &fakeGitHub{marker: "<!-- closes-as: duplicate -->", author: "someone-else"}
	require.NoError(t, run(context.Background(), []string{"close", "1", "--duplicate-of", "2"}, f, true))
	require.Equal(t, 1, f.posts)
}

func TestAC1CloseArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{},
		{"open"},
		{"close", "1"},
		{"close", "1", "--duplicate-of", "2", "--superseded-by", "3"},
		{"close", "1", "--duplicate-of", "2", "3"},
		{"close", "1", "--superseded-by", "other/repo#2"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			f := &fakeGitHub{}
			require.Error(t, run(t.Context(), args, f, true))
			require.Empty(t, f.calls)
		})
	}
}
