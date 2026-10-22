// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
)

const (
	exitEnvironment = 2
	apiCommand      = "api"
	jqFlag          = "--jq"
)

var errEnvironment = errors.New("cannot resolve repository, read input, or execute gh")

// Run posts only after all text has passed the scan and identity guard.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	r, err := parse(args)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post:", errUsage)
		return exitEnvironment
	}
	repo, err := resolveRepo(ctx, r.repo)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post:", errEnvironment)
		return exitEnvironment
	}
	r.repo = repo
	body, err := readBody(r.bodyFile, in)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot read body")
		return exitEnvironment
	}
	config, err := settings(repo)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post:", errConfig)
		return exitEnvironment
	}
	if config["scan_mode"] != "" && config["scan_mode"] != "all" && config["scan_mode"] != secretsOnlyMode {
		fmt.Fprintln(errOut, "gh-post: invalid scan_mode")
		return exitEnvironment
	}
	private, err := scanConfig(ctx, config)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post:", errConfig)
		return exitEnvironment
	}

	texts := append([]string{string(body), r.operand}, r.text...)
	refused := false
	for _, text := range texts {
		for _, finding := range Scan(text, private, config["scan_mode"] == secretsOnlyMode) {
			fmt.Fprintf(errOut, "gh-post: %s line %d\n", finding.Category, finding.Line)
			refused = true
		}
	}
	if refused {
		return 1
	}
	return post(ctx, r, body, config["post_identities"], out, errOut)
}

func scanConfig(ctx context.Context, config map[string]string) ([]string, error) {
	switch config["private_strings"] {
	case "", "optional", requiredList, "none":
	default:
		return nil, errConfig
	}
	if config["scan_mode"] == secretsOnlyMode {
		return nil, nil
	}
	return privateStrings(ctx, config)
}

func readBody(path string, in io.Reader) ([]byte, error) {
	if path != "" && path != "-" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, errEnvironment
		}
		return data, nil
	}
	data, err := io.ReadAll(in)
	if err != nil {
		return nil, errEnvironment
	}
	return data, nil
}

func resolveRepo(ctx context.Context, repo string) (string, error) {
	if repo == "" {
		data, err := exec.CommandContext(ctx, "git", "remote", "get-url", "origin").Output()
		if err != nil {
			return "", errEnvironment
		}
		repo = strings.TrimSpace(string(data))
		for _, prefix := range []string{"git@github.com:", "ssh://git@github.com/", "https://github.com/"} {
			repo = strings.TrimPrefix(repo, prefix)
		}
		repo = strings.TrimSuffix(repo, ".git")
	}
	repo = strings.TrimPrefix(repo, "github.com/")
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(repo) || strings.Contains(repo, "..") {
		return "", errEnvironment
	}
	return repo, nil
}

func gh(ctx context.Context, args []string, input []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Stdin = bytes.NewReader(input)
	// Explicit targets and body input must not be redirected.
	cmd.Env = cleanEnv(os.Environ())
	data, err := cmd.Output()
	if err != nil {
		return nil, errEnvironment
	}
	return data, nil
}

func cleanEnv(env []string) []string {
	result := make([]string, 0, len(env)+operationWords)
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "GH_REPO", "GH_HOST", "GH_DEBUG", "GH_PROMPT_DISABLED", "GH_PAGER":
			continue
		}
		result = append(result, entry)
	}
	return append(result, "GH_PROMPT_DISABLED=1", "GH_PAGER=cat")
}

func identityAllowed(ctx context.Context, allowlist string) (bool, error) {
	identities := splitList(allowlist)
	if len(identities) == 0 {
		return false, nil
	}
	data, err := gh(ctx, []string{apiCommand, "user", jqFlag, ".login"}, nil)
	if err != nil {
		data, err = gh(
			ctx,
			[]string{apiCommand, "graphql", "-f", "query={viewer{login}}", jqFlag, ".data.viewer.login"},
			nil,
		)
	}
	if err != nil {
		return false, err
	}
	login := strings.TrimSpace(string(data))
	return slices.Contains(identities, login), nil
}

func post(ctx context.Context, r request, body []byte, identities string, out, errOut io.Writer) int {
	allowed, err := identityAllowed(ctx, identities)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot read gh identity")
		return exitEnvironment
	}
	if !allowed {
		fmt.Fprintln(errOut, "gh-post: identity refused")
		return 1
	}
	log, err := openAudit()
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot open audit log")
		return exitEnvironment
	}
	defer log.Close()
	args, input, err := r.command(body)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot encode body")
		return exitEnvironment
	}
	data, err := gh(ctx, args, input)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: gh write failed; check GitHub before retrying")
		return exitEnvironment
	}
	target := r.targetURL(ctx, string(data))
	if err = appendAudit(log, r.operation, target, body); err != nil {
		fmt.Fprintln(errOut, "gh-post: posted but audit append failed; do not retry")
		return exitEnvironment
	}
	fmt.Fprintln(out, target)
	return 0
}

func (r *request) command(body []byte) ([]string, []byte, error) {
	if r.operation == commentEdit || r.operation == reviewEdit {
		method := "PATCH"
		endpoint := "repos/" + r.repo + "/issues/comments/" + r.operand
		if r.kind == "review" {
			endpoint = "repos/" + r.repo + "/pulls/comments/" + r.operand
		}
		if r.operation == reviewEdit {
			method = "PUT"
			endpoint = "repos/" + r.repo + "/pulls/" + r.operand + "/reviews/" + r.reviewID
		}
		input, err := json.Marshal(struct {
			Body string `json:"body"`
		}{string(body)})
		if err != nil {
			return nil, nil, fmt.Errorf("encode body: %w", err)
		}
		return []string{apiCommand, "--method", method, endpoint, "--input", "-", jqFlag, ".html_url"}, input, nil
	}
	args := strings.Fields(r.operation)
	if r.operand != "" {
		args = append(args, r.operand)
	}
	args = append(args, "--repo", r.repo)
	args = append(args, r.flags...)
	// Metadata-only PR edits must preserve the existing description. An
	// explicit body file still allows callers to clear it deliberately.
	if r.operation == "pr edit" && r.bodyFile == "" && len(body) == 0 {
		return args, nil, nil
	}
	bodyFlag := bodyFileFlag
	if strings.HasPrefix(r.operation, "release ") {
		bodyFlag = "--notes-file"
	}
	return append(args, bodyFlag, "-"), body, nil
}

func (r *request) targetURL(ctx context.Context, output string) string {
	prefix := "https://github.com/" + r.repo + "/"
	for word := range strings.FieldsSeq(output) {
		if strings.HasPrefix(word, prefix) {
			return word
		}
	}
	// Review and release edits can be silent on success. Read the target URL.
	kind := strings.Fields(r.operation)[0]
	if kind == "issue" || kind == "pr" || kind == "release" {
		data, err := gh(ctx, []string{kind, "view", r.operand, "--repo", r.repo, "--json", "url", jqFlag, ".url"}, nil)
		if err == nil && strings.HasPrefix(strings.TrimSpace(string(data)), prefix) {
			return strings.TrimSpace(string(data))
		}
	}
	return strings.TrimSuffix(prefix, "/")
}
