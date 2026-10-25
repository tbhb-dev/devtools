// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var (
	errGrant = errors.New("close requires a granted confirmation from the skill shim")
	errUsage = errors.New("use close <number> --superseded-by <n>... | --duplicate-of <n> [--reason <file>]")
	errState = errors.New("target must be open and references must be open or merged in the same repository")
)

type githubClient interface {
	output(ctx context.Context, name string, args []string, input string) ([]byte, error)
}

type closeRequest struct {
	target string
	refs   []string
	mode   string
	reason string
}

func run(ctx context.Context, args []string, client githubClient, granted bool) error {
	if !granted {
		return errGrant
	}
	request, err := parseClose(args)
	if err != nil {
		return err
	}
	return request.execute(ctx, client)
}

func parseClose(args []string) (closeRequest, error) {
	var request closeRequest
	var superseding []string
	var duplicate string
	root := &cobra.Command{Use: "prctl", SilenceUsage: true, SilenceErrors: true}
	closeCmd := &cobra.Command{Use: "close", RunE: func(_ *cobra.Command, operands []string) error {
		if len(operands) == 0 || (len(superseding) == 0) == (duplicate == "") {
			return errUsage
		}
		request.target = operands[0]
		request.mode = "superseded"
		request.refs = append(request.refs, superseding...)
		request.refs = append(request.refs, operands[1:]...)
		if duplicate != "" {
			if len(operands) != 1 {
				return errUsage
			}
			request.mode = "duplicate"
			request.refs = []string{duplicate}
		}
		return request.validateNumbers()
	}}
	closeCmd.Flags().StringSliceVar(&superseding, "superseded-by", nil, "Replacement pull request numbers")
	closeCmd.Flags().StringVar(&duplicate, "duplicate-of", "", "Original pull request number")
	closeCmd.Flags().StringVar(&request.reason, "reason", "", "Reason file to lint and include")
	root.AddCommand(closeCmd)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		return request, fmt.Errorf("%w: %w", errUsage, err)
	}
	if request.target == "" {
		return request, errUsage
	}
	return request, nil
}

func (r closeRequest) execute(ctx context.Context, client githubClient) error {
	repoBytes, err := client.output(
		ctx,
		"gh",
		[]string{"repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner"},
		"",
	)
	if err != nil {
		return err
	}
	repo := strings.TrimSpace(string(repoBytes))
	found, err := readSnapshot(ctx, client, repo, r)
	if err != nil {
		return err
	}
	marker := "<!-- closes-as: " + r.mode + " -->"
	if !found.hasMarker(marker) {
		comment, commentErr := r.comment(ctx, client, repo, marker)
		if commentErr != nil {
			return commentErr
		}
		// gh-post scans the entire comment before posting, including the reason.
		if _, err = client.output(
			ctx,
			"gh-post",
			[]string{"pr", "comment", r.target, "--repo", repo, "--body-file", "-"},
			comment,
		); err != nil {
			return err
		}
	}
	_, err = client.output(ctx, "gh", []string{"pr", "close", r.target, "--repo", repo}, "")
	return err
}

func (r closeRequest) comment(ctx context.Context, client githubClient, repo, marker string) (string, error) {
	refs := make([]string, 0, len(r.refs))
	for _, number := range r.refs {
		refs = append(refs, repo+"#"+number)
	}
	prefix := "Superseded by "
	if r.mode == "duplicate" {
		prefix = "Duplicate of "
	}
	body := prefix + strings.Join(refs, ", ") + ".\n\n"
	if r.reason != "" {
		if _, err := client.output(ctx, "mise", []string{"run", "lint-draft", r.reason}, ""); err != nil {
			return "", err
		}
		reason, err := os.ReadFile(r.reason)
		if err != nil {
			return "", fmt.Errorf("read reason: %w", err)
		}
		body += strings.TrimSpace(string(reason)) + "\n\n"
	}
	return body + marker, nil
}

func (r closeRequest) validateNumbers() error {
	for _, number := range append([]string{r.target}, r.refs...) {
		n, err := strconv.Atoi(number)
		if err != nil || n <= 0 || strconv.Itoa(n) != number {
			return errUsage
		}
	}
	return nil
}
