// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	watchInconclusive       = 2
	watchQuotaStatus        = 76
	watchAppearanceInterval = 5
	watchDefaultInterval    = 30
	watchDefaultTimeout     = 1800
	watchDefaultAppearance  = 120
	watchNormalizeError     = "normalize"
)

type watchedCheck struct {
	Name     string `json:"name"`
	Bucket   string `json:"bucket"`
	Link     string `json:"link"`
	Workflow string `json:"workflow"`
}
type checkWatcher struct {
	out, stderr                   io.Writer
	read                          func() ([]byte, string, int)
	sleep                         func(int)
	interval, timeout, appearance int
}

func watchCommand(code *int) *cobra.Command {
	return &cobra.Command{
		Use: "watch [pull-request-number]", Short: "Stream settled pull request checks until completion",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// The capability probe must work without Git, `gh`, or repository access.
			if len(args) == 1 && args[0] == "--help" {
				return cmd.Help()
			}
			*code = watchChecks(cmd, args)
			return nil
		},
	}
}

func watchChecks(cmd *cobra.Command, args []string) int {
	env := diagnosisEnv()
	run := func(dir, name string, args ...string) ([]byte, string, int) {
		child := exec.CommandContext(cmd.Context(), name, args...)
		child.Dir = dir
		child.Env = env
		var stderr bytes.Buffer
		child.Stderr = &stderr
		out, err := child.Output()
		status := 0
		if err != nil {
			status = 1
			if exit, ok := errors.AsType[*exec.ExitError](err); ok {
				status = exit.ExitCode()
			} else {
				fmt.Fprintln(&stderr, err)
			}
		}
		return out, stderr.String(), status
	}
	root, diagnostic, status := run("", "git", "rev-parse", "--show-toplevel")
	if status != 0 {
		fmt.Fprint(cmd.ErrOrStderr(), diagnostic)
		return watchInconclusive
	}
	dir := strings.TrimSpace(string(root))
	if _, err := exec.LookPath("gh"); err != nil {
		cmd.Println("ERROR gh is not installed")
		return watchInconclusive
	}
	number, gh, selectionStatus := selectWatchTarget(
		cmd,
		args,
		func(args ...string) ([]byte, string, int) { return run(dir, "gh", args...) },
		func() ([]byte, string, int) { return run(dir, "git", "rev-parse", "--abbrev-ref", "HEAD") },
	)
	if selectionStatus != 0 {
		return selectionStatus
	}

	w := checkWatcher{
		out:        cmd.OutOrStdout(),
		stderr:     cmd.ErrOrStderr(),
		interval:   watchSetting("PR_CHECKS_INTERVAL", watchDefaultInterval),
		timeout:    watchSetting("PR_CHECKS_TIMEOUT", watchDefaultTimeout),
		appearance: watchSetting("PR_CHECKS_APPEAR_TIMEOUT", watchDefaultAppearance),
		sleep:      func(n int) { time.Sleep(time.Duration(n) * time.Second) },
		read:       func() ([]byte, string, int) { return gh("pr", "checks", number, "--json", "name,bucket,link,workflow") },
	}
	return w.watch(number)
}

func watchSetting(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
	}
	return fallback
}

// watchQuota classifies only a failed command's stderr while ordinary errors
// keep their original status. It returns the shell reader's guidance verbatim
// without issuing another request.
func watchQuota(diagnostic string) string {
	for line := range strings.SplitSeq(diagnostic, "\n") {
		if !strings.HasPrefix(line, "gh: ") && !strings.HasPrefix(line, "GraphQL: ") {
			continue
		}
		if strings.Contains(line, "secondary rate limit") {
			return "rate limit: GitHub secondary rate limit; the reset time isn't known from the failure. Wait at least one minute before the next request, as GitHub's docs advise; no budget query helps. No automatic retry was made.\n"
		}
		if strings.Contains(line, "API rate limit exceeded for") ||
			strings.Contains(line, "API rate limit already exceeded for") {
			return "rate limit: GitHub primary GraphQL rate limit, inferred from the command and the error line; the reset time isn't known from the failure. Once the limit has passed, read the budget with a GraphQL rateLimit { remaining resetAt } query, never /rate_limit. No automatic retry was made.\n"
		}
	}
	return ""
}

var watchRunID = regexp.MustCompile(`[0-9]+$`)

func normalizeWatch(data []byte) ([]watchedCheck, string) {
	var raw []json.RawMessage
	if len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '[' || json.Unmarshal(data, &raw) != nil {
		return nil, "invalid"
	}
	type attempt struct {
		check watchedCheck
		id    float64
	}
	attempts := make([]attempt, 0, len(raw))
	for _, entry := range raw {
		check, id, err := normalizeWatchEntry(entry)
		if err != nil {
			return nil, watchNormalizeError
		}
		attempts = append(attempts, attempt{check, id})
	}

	sort.SliceStable(attempts, func(i, j int) bool {
		a, b := attempts[i], attempts[j]
		if comparison := compareWatchPair(a.check, b.check); comparison != 0 {
			return comparison < 0
		}
		return a.id < b.id
	})
	result := make([]watchedCheck, 0, len(attempts))
	for _, a := range attempts {
		if len(result) > 0 && result[len(result)-1].Workflow == a.check.Workflow &&
			result[len(result)-1].Name == a.check.Name {
			result[len(result)-1] = a.check
		} else {
			result = append(result, a.check)
		}
	}
	return result, ""
}

func (w checkWatcher) snapshot(number string) ([]watchedCheck, int) {
	data, diagnostic, status := w.read()
	if status != 0 {
		if status == watchQuotaStatus {
			fmt.Fprint(w.stderr, diagnostic)
			return nil, watchQuotaStatus
		}
		if !strings.Contains(diagnostic, "no checks reported") {
			first, _, _ := strings.Cut(diagnostic, "\n")
			fmt.Fprintf(w.out, "ERROR could not read checks for #%s: %s\n", number, first)
			return nil, watchInconclusive
		}
		data = []byte("[]")
	}
	checks, problem := normalizeWatch(data)
	switch problem {
	case "invalid":
		fmt.Fprintf(w.out, "ERROR gh returned an invalid checks response for #%s\n", number)
		return nil, watchInconclusive
	case watchNormalizeError:
		fmt.Fprintf(w.out, "ERROR could not normalize checks for #%s\n", number)
		return nil, watchInconclusive
	}
	return checks, 0
}

// watch keeps appearance and settling on separate clocks. Each phase reads its
// own snapshot, including the reads between phases, to match the shell sequence.
// A zero appearance bound skips only the appearance loop.
func (w checkWatcher) watch(number string) int {
	fmt.Fprintf(w.out, "WATCHING #%s\n", number)
	for waited := 0; waited < w.appearance; waited += watchAppearanceInterval {
		checks, status := w.snapshot(number)
		if status != 0 {
			return status
		}
		if len(checks) > 0 {
			break
		}
		w.sleep(watchAppearanceInterval)
	}
	checks, status := w.snapshot(number)
	if status != 0 {
		return status
	}
	if len(checks) == 0 {
		fmt.Fprintf(w.out, "NO CHECKS #%s registered after %ds\n", number, w.appearance)
		return watchInconclusive
	}
	seen := ""
	for elapsed := 0; ; elapsed += w.interval {
		checks, status = w.snapshot(number)
		if status != 0 {
			return status
		}
		current, pending := watchEvents(checks)
		seen = w.emit(current, seen)

		if pending == 0 && len(checks) > 0 {
			return w.finish(number, current)
		}
		if elapsed >= w.timeout {
			fmt.Fprintf(w.out, "TIMEOUT #%s after %ds with checks still running\n", number, w.timeout)
			return watchInconclusive
		}
		w.sleep(w.interval)
	}
}

// watchEvents defers cancellations until pending work finishes. Sort complete
// event rows by bytes so the stream follows LC_ALL=C regardless of host locale.
func watchEvents(checks []watchedCheck) ([]string, int) {
	pending := 0
	for _, check := range checks {
		if check.Bucket == "pending" {
			pending++
		}
	}
	var current []string
	for _, check := range checks {
		if check.Bucket == "pending" || check.Bucket == "cancel" && pending != 0 {
			continue
		}
		current = append(current, check.Bucket+"\t"+check.Workflow+":"+check.Name+"\t"+check.Name+"\t"+check.Link)
	}
	sort.Strings(current)
	return current, pending
}

func (w checkWatcher) emit(current []string, seen string) string {
	for _, line := range current {
		fields := strings.Split(line, "\t")
		bucket, key, name, link := fields[0], fields[1], fields[2], fields[3]
		if name == "" || strings.Contains(seen, "|"+key+"|") {
			continue
		}
		seen += "|" + key + "|"
		switch bucket {
		case "pass":
			fmt.Fprintf(w.out, "PASS  %s\n", name)
		case "skipping":
			fmt.Fprintf(w.out, "SKIP  %s\n", name)
		case "fail", "cancel":
			fmt.Fprintf(w.out, "FAIL  %s  %s\n", name, link)
		default:
			fmt.Fprintf(w.out, "%s  %s\n", bucket, name)
		}
	}
	return seen
}

// finish recounts the final snapshot. Earlier events stay in the stream even
// when a later attempt replaces their result.
func (w checkWatcher) finish(number string, current []string) int {
	failures := 0
	for _, line := range current {
		if strings.HasPrefix(line, "fail") || strings.HasPrefix(line, "cancel") {
			failures++
		}
	}
	if failures == 0 {
		fmt.Fprintf(w.out, "ALL GREEN #%s\n", number)
		return 0
	}
	fmt.Fprintf(w.out, "FAILED #%s with %d failing check(s). Hand it to the fix-pr skill.\n", number, failures)
	return 1
}

type watchReader func(args ...string) ([]byte, string, int)

func selectWatchTarget(
	cmd *cobra.Command,
	args []string,
	raw watchReader,
	currentBranch func() ([]byte, string, int),
) (string, watchReader, int) {
	gh := func(args ...string) ([]byte, string, int) {
		data, diagnostic, status := raw(args...)
		if status != 0 {
			if guidance := watchQuota(diagnostic); guidance != "" {
				diagnostic = guidance + diagnostic
				status = watchQuotaStatus
			}
		}
		return data, diagnostic, status
	}
	login, diagnostic, status := gh(
		"api",
		"graphql",
		"-f",
		"query={viewer{login}}",
		"--jq",
		".data.viewer.login // empty",
	)
	fmt.Fprint(cmd.ErrOrStderr(), diagnostic)
	if status != 0 {
		return "", nil, status
	}
	if value := strings.TrimSpace(string(login)); value == "" || value == "null" {
		cmd.PrintErrln("GitHub returned no viewer login")
		return "", nil, 1
	}
	number := ""
	if len(args) > 0 {
		number = args[0]
	}
	if number == "" {
		branch, branchDiagnostic, _ := currentBranch()
		fmt.Fprint(cmd.ErrOrStderr(), branchDiagnostic)
		name := strings.TrimSpace(string(branch))
		data, discoveryDiagnostic, discoveryStatus := gh(
			"pr",
			"list",
			"--head",
			name,
			"--state",
			"open",
			"--json",
			"number",
			"--jq",
			".[0].number",
		)
		fmt.Fprint(cmd.ErrOrStderr(), discoveryDiagnostic)
		if discoveryStatus != 0 {
			return "", nil, discoveryStatus
		}
		number = strings.TrimSpace(string(data))
		if number == "" {
			cmd.Printf("ERROR no open pull request for %s\n", name)
			return "", nil, watchInconclusive
		}
	}
	return number, gh, 0
}

func compareWatchPair(a, b watchedCheck) int {
	if n := strings.Compare(a.Workflow, b.Workflow); n != 0 {
		return n
	}
	return strings.Compare(a.Name, b.Name)
}

func normalizeWatchEntry(entry json.RawMessage) (watchedCheck, float64, error) {
	var check watchedCheck
	if err := json.Unmarshal(entry, &check); err != nil {
		return check, 0, fmt.Errorf("decode check: %w", err)
	}
	match := watchRunID.FindString(check.Link)
	// Links without a trailing numeric suffix sort at zero.
	if match == "" {
		return check, 0, nil
	}
	id, err := strconv.ParseFloat(match, 64)
	if err != nil {
		return check, 0, fmt.Errorf("decode run ID: %w", err)
	}
	return check, id, nil
}
