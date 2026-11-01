// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const runRerunFailed = "run rerun-failed"

var errRerunReserved = errors.New("rerun already reserved for this run attempt; do not retry")

// This is the entire run-command allowlist: no aliases, inferred target,
// extra flags, full rerun, or cancellation can reach the write path.
func parseRerunFailed(args []string) (request, error) {
	if len(args) != 5 || args[1] != "rerun-failed" || args[3] != repoFlag ||
		!regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(args[2]) ||
		!regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(args[4]) {
		return request{}, errUsage
	}
	if _, err := strconv.ParseUint(args[2], 10, 64); err != nil {
		return request{}, errUsage
	}
	return request{operation: runRerunFailed, operand: args[2], repo: args[4]}, nil
}

type workflowRun struct {
	ID         uint64 `json:"id"`
	Attempt    uint64 `json:"run_attempt"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	URL        string `json:"html_url"`
}

func (r *request) runURL() string {
	return "https://github.com/" + strings.TrimPrefix(r.runEndpoint(), "repos/")
}

func (r *request) runEndpoint() string {
	return "repos/" + r.repo + "/actions/runs/" + r.operand
}

func (r *request) readRun(ctx context.Context) (workflowRun, error) {
	data, err := gh(ctx, []string{apiCommand, methodFlag, "GET", r.runEndpoint()})
	var state workflowRun
	if err != nil || json.Unmarshal(data, &state) != nil || state.Attempt == 0 || state.Status == "" ||
		strconv.FormatUint(state.ID, 10) != r.operand || !strings.EqualFold(state.URL, r.runURL()) {
		return workflowRun{}, errEnvironment
	}
	return state, nil
}

func printRunState(out io.Writer, state workflowRun) {
	fmt.Fprintf(out, "status=%q conclusion=%q attempt=%d", state.Status, state.Conclusion, state.Attempt)
}

func (r *request) rerunFailed(ctx context.Context, log *os.File, out, errOut io.Writer) int {
	state, err := r.readRun(ctx)
	if err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot read exact workflow run")
		return exitEnvironment
	}
	if state.Status != "completed" ||
		(state.Conclusion != "failure" && state.Conclusion != "cancelled" && state.Conclusion != "timed_out") {
		fmt.Fprint(errOut, "gh-post: run refused: ")
		printRunState(errOut, state)
		fmt.Fprintln(errOut)
		return 1
	}
	if err = r.reserveRerun(filepath.Dir(log.Name()), state.Attempt); err != nil {
		if errors.Is(err, errRerunReserved) {
			fmt.Fprintln(errOut, "gh-post:", err)
			return 1
		}
		fmt.Fprintln(errOut, "gh-post: cannot reserve run attempt; no rerun requested")
		return exitEnvironment
	}
	if err = r.auditRerun(log, state.Attempt); err != nil {
		fmt.Fprintln(errOut, "gh-post: cannot audit reservation; no rerun requested; do not retry this attempt")
		return exitEnvironment
	}
	_, err = gh(ctx, []string{apiCommand, methodFlag, "POST", r.runEndpoint() + "/rerun-failed-jobs"})
	if err != nil {
		// A transport failure can follow acceptance. Keep the reservation,
		// read once for evidence, and never repeat the POST.
		after, readErr := r.readRun(ctx)
		fmt.Fprint(errOut, "gh-post: rerun outcome uncertain; do not retry this attempt; ")
		if readErr != nil {
			fmt.Fprintln(errOut, "cannot read run back")
		} else {
			printRunState(errOut, after)
			fmt.Fprintln(errOut)
		}
		return exitEnvironment
	}
	fmt.Fprintln(out, r.runURL())
	return 0
}

func (r *request) reserveRerun(dir string, attempt uint64) error {
	dir = filepath.Join(dir, "gh-post-reruns", strings.ToLower(r.repo), r.operand)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("reserve directory: %w", err)
	}
	// O_EXCL prevents concurrent processes from reserving the same attempt,
	// including after an uncertain result. Keep receipts after write or sync failures.
	file, err := os.OpenFile(
		filepath.Join(dir, strconv.FormatUint(attempt, 10)),
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0o600,
	)
	if errors.Is(err, os.ErrExist) {
		return errRerunReserved
	}
	if err != nil {
		return fmt.Errorf("reserve attempt: %w", err)
	}
	defer file.Close()
	if _, err = file.WriteString("reserved\n"); err != nil {
		return fmt.Errorf("write reservation: %w", err)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("sync reservation: %w", err)
	}
	return nil
}

func (r *request) auditRerun(log *os.File, attempt uint64) error {
	record := struct {
		Time      string `json:"time"`
		Operation string `json:"operation"`
		URL       string `json:"url"`
		Attempt   uint64 `json:"run_attempt"`
		Outcome   string `json:"outcome"`
	}{time.Now().UTC().Format(time.RFC3339Nano), r.operation, r.runURL(), attempt, "reserved"}
	if err := json.NewEncoder(log).Encode(record); err != nil {
		return fmt.Errorf("audit reservation: %w", err)
	}
	if err := log.Sync(); err != nil {
		return fmt.Errorf("sync audit: %w", err)
	}
	return nil
}
