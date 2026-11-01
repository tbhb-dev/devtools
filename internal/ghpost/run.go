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
	"unicode/utf16"
	"unicode/utf8"
)

const (
	exitEnvironment = 2
	// maxWriteErrorBytes bounds the GitHub error text echoed after a failed write.
	maxWriteErrorBytes = 2000
	apiCommand         = "api"
	jqFlag             = "--jq"
	jsonFlag           = "--json"
	methodFlag         = "--method"
	viewVerb           = "view"
	graphqlCommand     = "graphql"
)

var errEnvironment = errors.New("cannot resolve repository, read input, or execute gh")

// Run posts only after all text has passed the scan and identity guard.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	if code, handled := informational(args, out, errOut); handled {
		return code
	}
	r, err := parse(args)
	finish, auditErr := r.startTypeAudit(errOut)
	if auditErr != nil {
		fmt.Fprintln(errOut, "gh-post: cannot open audit log")
		return exitEnvironment
	}
	code := runRequest(ctx, &r, err, in, out, errOut)
	finish(&code)
	return code
}

func runRequest(ctx context.Context, r *request, err error, in io.Reader, out, errOut io.Writer) int {
	if err != nil {
		fmt.Fprintln(errOut, "gh-post:", parseFailure(err))
		return exitEnvironment
	}
	repo, err := resolveRepo(ctx, r.repo)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post:", errEnvironment)
		return exitEnvironment
	}
	r.repo = repo
	body, err := r.readInput(in)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post:", err)
		return exitEnvironment
	}
	if err = r.completeBody(body); err != nil {
		fmt.Fprintln(errOut, "gh-post:", err)
		return 1
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
	r.private = private
	r.secretsOnly = config["scan_mode"] == secretsOnlyMode
	return post(ctx, *r, body, config["post_identities"], out, errOut)
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

func gh(ctx context.Context, args []string) ([]byte, error) {
	data, _, err := ghRun(ctx, args, nil)
	return data, err
}

// ghRun also returns stderr, for callers that tell a missing target from a failure.
func ghRun(ctx context.Context, args []string, input []byte) ([]byte, string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Stdin = bytes.NewReader(input)
	// Explicit targets and body input must not be redirected.
	cmd.Env = cleanEnv(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), stderr.String(), errEnvironment
	}
	return stdout.Bytes(), stderr.String(), nil
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

func identityAllowed(ctx context.Context, allowlist string) (string, bool, error) {
	identities := splitList(allowlist)
	if len(identities) == 0 {
		return "", false, nil
	}
	data, err := gh(ctx, []string{apiCommand, "user", jqFlag, ".login"})
	if err != nil {
		data, err = gh(
			ctx,
			[]string{apiCommand, graphqlCommand, "-f", "query={viewer{login}}", jqFlag, ".data.viewer.login"},
		)
	}
	if err != nil {
		return "", false, err
	}
	login := strings.TrimSpace(string(data))
	return login, slices.Contains(identities, login), nil
}

func post(ctx context.Context, r request, body []byte, identities string, out, errOut io.Writer) int {
	login, allowed, err := identityAllowed(ctx, identities)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot read gh identity")
		return exitEnvironment
	}
	if !allowed {
		fmt.Fprintln(errOut, "gh-post: identity refused")
		return 1
	}
	if r.issueType != "" {
		return r.changeIssueType(ctx, body, out, errOut)
	}
	log, err := openAudit()
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot open audit log")
		return exitEnvironment
	}
	defer log.Close()
	if r.operation == runRerunFailed {
		return r.rerunFailed(ctx, log, out, errOut, (*os.File).Sync)
	}
	if r.isIssueStateChange() {
		return r.changeIssueState(ctx, body, log, out, errOut)
	}
	if r.isPRStateChange() {
		return r.changePRState(ctx, body, login, log, out, errOut)
	}
	if r.isLabelChange() {
		return r.changeLabels(ctx, log, out, errOut)
	}
	return postContent(ctx, r, body, log, out, errOut)
}

func postContent(ctx context.Context, r request, body []byte, log, out, errOut io.Writer) int {
	if r.operation == issueEdit && len(body) > 0 {
		if code := r.checkIssueBodyTarget(ctx, errOut); code != 0 {
			return code
		}
	}
	args, input, err := r.command(body)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot encode body")
		return exitEnvironment
	}
	if r.operation == prReview {
		if code := r.checkReviewHead(ctx, errOut); code != 0 {
			return code
		}
	}
	data, stderr, err := ghRun(ctx, args, input)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: gh write failed; check GitHub before retrying")
		r.printWriteError(stderr, body, errOut)
		r.printWriteError(errorBody(data), body, errOut)
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

func errorBody(data []byte) string {
	var fields struct {
		Message json.RawMessage `json:"message,omitempty"`
		Errors  json.RawMessage `json:"errors,omitempty"`
	}
	if strings.HasPrefix(strings.TrimSpace(string(data)), "{") && json.Unmarshal(data, &fields) == nil {
		if fields.Message == nil && fields.Errors == nil {
			return ""
		}
		if encoded, err := json.Marshal(fields); err == nil {
			data = encoded
		}
	}
	return string(data)
}

func (r *request) printWriteError(text string, body []byte, errOut io.Writer) {
	if len(body) > 0 {
		text = removeErrorBody(text, string(body))
	}
	findings := Scan(text, r.private, r.secretsOnly)
	for _, finding := range findings {
		fmt.Fprintf(errOut, "gh-post: %s line %d\n", finding.Category, finding.Line)
	}
	if len(findings) > 0 {
		return
	}
	if len(text) > maxWriteErrorBytes {
		end := maxWriteErrorBytes
		for !utf8.RuneStart(text[end]) {
			end--
		}
		text = text[:end]
	}
	if text != "" {
		fmt.Fprint(errOut, text)
		if !strings.HasSuffix(text, "\n") {
			fmt.Fprintln(errOut)
		}
	}
}

func removeErrorBody(text, body string) string {
	if body == "" {
		return text
	}
	var pattern strings.Builder
	for _, char := range body {
		pattern.WriteString(`(?:(?i:`)
		if utf16.RuneLen(char) > 1 {
			high, low := utf16.EncodeRune(char)
			fmt.Fprintf(&pattern, `\\+u%04x\\+u%04x`, high, low)
		} else {
			fmt.Fprintf(&pattern, `\\+u%04x`, char)
		}
		pattern.WriteString(`)|`)
		if index := strings.IndexRune("\b\f\n\r\t\"\\/", char); index >= 0 {
			escape := []string{"b", "f", "n", "r", "t", `"`, `\`, `/`}[index]
			pattern.WriteString(`\\+` + regexp.QuoteMeta(escape) + `|`)
		}
		pattern.WriteString(regexp.QuoteMeta(string(char)) + `)`)
	}
	return regexp.MustCompile(pattern.String()).ReplaceAllString(text, "")
}

func (r *request) command(body []byte) ([]string, []byte, error) {
	if r.operation == prReview {
		return r.reviewCommand(body)
	}
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
		return []string{apiCommand, methodFlag, method, endpoint, "--input", "-", jqFlag, ".html_url"}, input, nil
	}
	args := strings.Fields(r.operation)
	if r.operand != "" {
		args = append(args, r.operand)
	}
	args = append(args, repoFlag, r.repo)
	args = append(args, r.flags...)
	// Metadata-only edits must preserve the existing description.
	keepsBody := r.operation == "pr edit" || r.operation == issueEdit
	if keepsBody && r.bodyFile == "" && len(body) == 0 {
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
	if r.operation == issueEdit {
		return prefix + "issues/" + r.operand
	}
	for word := range strings.FieldsSeq(output) {
		if strings.HasPrefix(word, prefix) {
			return word
		}
	}
	// Review and release edits can be silent on success. Read the target URL.
	kind := strings.Fields(r.operation)[0]
	if kind == issueVerb || kind == "pr" || kind == "release" {
		data, err := gh(
			ctx,
			[]string{kind, viewVerb, r.operand, repoFlag, r.repo, jsonFlag, "url", jqFlag, ".url"},
		)
		if err == nil && strings.HasPrefix(strings.TrimSpace(string(data)), prefix) {
			return strings.TrimSpace(string(data))
		}
	}
	return strings.TrimSuffix(prefix, "/")
}
