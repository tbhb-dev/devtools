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
	"time"
)

var errIssueType = errors.New("--type requires Feature, Defect, Chore or Spike")

func (r *request) addIssueType(value string) error {
	if r.issueType != "" {
		return errIssueType
	}
	r.issueType = value
	if !slices.Contains([]string{"Feature", "Defect", "Chore", "Spike"}, value) {
		return errIssueType
	}
	return nil
}

func (r *request) startTypeAudit(errOut io.Writer) (func(*int), error) {
	if !r.typeSet {
		return func(*int) {}, nil
	}
	log, err := openAudit()
	if err != nil {
		return nil, err
	}
	return func(code *int) {
		defer log.Close()
		if appendErr := r.auditIssueType(log, *code); appendErr != nil {
			fmt.Fprintln(errOut, "gh-post: audit append failed; check GitHub before retrying")
			*code = exitEnvironment
		}
	}, nil
}

type issueType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type typedIssue struct {
	ID              string    `json:"id"`
	ViewerCanUpdate bool      `json:"viewerCanUpdateMetadata"`
	Type            issueType `json:"issueType"`
}

func (r *request) auditIssueType(log io.Writer, code int) error {
	outcome := "refused"
	if code == 0 {
		outcome = "success"
	}
	base := auditRecord{
		Time: time.Now().UTC().Format(time.RFC3339Nano), Operation: r.operation,
		URL: "https://github.com/" + r.repo + "/issues/" + r.operand,
	}
	record := struct {
		auditRecord

		Issue   string `json:"issue"`
		Type    string `json:"type"`
		Outcome string `json:"outcome"`
	}{
		auditRecord: base,
		Issue:       r.operand, Type: r.issueType, Outcome: outcome,
	}
	if err := json.NewEncoder(log).Encode(record); err != nil {
		return fmt.Errorf("audit issue type: %w", err)
	}
	return nil
}

func (r *request) typeQuery(ctx context.Context, field, filter string) ([]byte, error) {
	owner, name, _ := strings.Cut(r.repo, "/")
	return gh(ctx, []string{
		apiCommand, graphqlCommand, "-f",
		"query=query($owner:String!,$name:String!){repository(owner:$owner,name:$name){" + field + "}}",
		"-f", "owner=" + owner, "-f", "name=" + name, jqFlag, ".data.repository." + filter,
	})
}

func (r *request) readTypedIssue(ctx context.Context) (typedIssue, error) {
	data, err := r.typeQuery(ctx, "issue(number:"+r.operand+"){id viewerCanUpdateMetadata issueType{id name}}", "issue")
	var issue typedIssue
	if err != nil || json.Unmarshal(data, &issue) != nil || issue.ID == "" {
		return issue, errEnvironment
	}
	return issue, nil
}

func (r *request) resolveIssueType(ctx context.Context) (string, error) {
	data, err := r.typeQuery(ctx, "issueTypes(first:100){nodes{id name}}", "issueTypes.nodes")
	var types []issueType
	if err != nil || json.Unmarshal(data, &types) != nil {
		return "", errEnvironment
	}
	for _, kind := range types {
		if kind.Name == r.issueType && kind.ID != "" {
			return kind.ID, nil
		}
	}
	return "", errEnvironment
}

func (r *request) changeIssueType(ctx context.Context, body []byte, out, errOut io.Writer) int {
	id, err := r.resolveIssueType(ctx)
	if err != nil {
		fmt.Fprintf(errOut, "gh-post: cannot resolve issue type %q in %s\n", r.issueType, r.repo)
		return exitEnvironment
	}
	issue, err := r.readTypedIssue(ctx)
	if err != nil {
		fmt.Fprintf(errOut, "gh-post: cannot read issue %s in %s\n", r.operand, r.repo)
		return exitEnvironment
	}
	if !issue.ViewerCanUpdate {
		fmt.Fprintf(errOut, "gh-post: cannot update issue %s in %s\n", r.operand, r.repo)
		return exitEnvironment
	}
	if r.bodyFile != "" || len(r.flags) > 0 {
		log, auditErr := openAudit()
		if auditErr != nil {
			fmt.Fprintln(errOut, "gh-post: cannot open audit log")
			return exitEnvironment
		}
		defer log.Close()
		if code := postContent(ctx, *r, body, log, out, errOut); code != 0 {
			return code
		}
	}
	if issue.Type.Name == r.issueType {
		fmt.Fprintln(out, "gh-post: issue type already as requested, nothing changed")
		return 0
	}
	args := []string{
		apiCommand, graphqlCommand, "-f",
		"query=mutation($id:ID!,$issueTypeId:ID!){updateIssue(input:{id:$id,issueTypeId:$issueTypeId}){issue{id}}}",
		"-f", "id=" + issue.ID, "-f", "issueTypeId=" + id,
	}
	data, stderr, writeErr := ghRun(ctx, args, nil)
	if writeErr != nil {
		fmt.Fprintln(errOut, "gh-post: issue type write failed; check GitHub before retrying")
		r.printWriteError(stderr, body, errOut)
		r.printWriteError(errorBody(data), body, errOut)
		return exitEnvironment
	}
	after, err := r.readTypedIssue(ctx)
	if err != nil || after.ID != issue.ID || after.Type.Name != r.issueType {
		fmt.Fprintln(errOut, "gh-post: cannot confirm issue type; check GitHub before retrying")
		return exitEnvironment
	}
	old := issue.Type.Name
	if old == "" {
		old = "untyped"
	}
	fmt.Fprintf(out, "gh-post: issue %s type changed from %s to %s\n", r.operand, old, after.Type.Name)
	return 0
}
