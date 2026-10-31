// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

// cspell:words CDPATH

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func diagnoseCommand(code *int) *cobra.Command {
	return &cobra.Command{
		Use:                "diagnose [pull-request-number]",
		Short:              "Report failing PR checks and suggest local reproducers",
		Long:               "Diagnose an explicit PR or the current branch's open PR without changing it. Exit codes: 0 no failing checks, 1 known failures, 2 unable to diagnose. Empty or unreadable checks are inconclusive. Reproducers are suggestions only.",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Cobra handles the capability probe without starting any subprocess.
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				return cmd.Help()
			}
			d := diagnosis{run: diagnosisRunner(cmd.Context()), out: cmd.OutOrStdout(), stderr: cmd.ErrOrStderr()}
			*code = d.diagnose(args, os.Getenv("FIX_PR_LOG_LINES"))
			return nil
		},
	}
}

type diagnosis struct {
	run         func(dir, tool string, args ...string) ([]byte, error)
	out, stderr io.Writer
}

// Pin the same report-affecting environment as the compatibility entry point.
func diagnosisEnv() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "LC_ALL", "GH_PAGER", "GH_PROMPT_DISABLED", "PYTHONUTF8", "CDPATH", "GH_REPO", "GH_HOST", "GREP_OPTIONS":
			continue
		}
		env = append(env, entry)
	}
	return append(env, "LC_ALL=C", "GH_PAGER=cat", "GH_PROMPT_DISABLED=1", "PYTHONUTF8=1")
}

func diagnosisRunner(ctx context.Context) func(string, string, ...string) ([]byte, error) {
	env := diagnosisEnv()
	return func(dir, tool string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, tool, args...)
		cmd.Dir, cmd.Env = dir, env
		return cmd.Output()
	}
}

type diagnosisPR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Branch string `json:"headRefName"`
	Head   string `json:"headRefOid"`
	URL    string `json:"url"`
}

type diagnosisCheck struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Link  string `json:"link"`
}

func (d diagnosis) refuse(message string) int {
	fmt.Fprintln(d.stderr, "diagnose: "+message)
	const inconclusive = 2
	return inconclusive
}

func (d diagnosis) diagnose(args []string, logLimit string) int {
	if len(args) > 1 {
		return d.refuse("usage: diagnose.sh [pull-request-number]")
	}
	root, branch, problem := d.checkout()
	if problem != "" {
		return d.refuse(problem)
	}
	number, problem := d.target(root, branch, args)
	if problem != "" {
		return d.refuse(problem)
	}
	fmt.Fprintln(d.out, "== pull request ==")
	data, err := d.run(root, "gh", "pr", "view", number, "--json", "number,title,headRefName,headRefOid,url")
	var pr diagnosisPR
	if err != nil || json.Unmarshal(data, &pr) != nil || !pr.valid() {
		return d.refuse("cannot read #" + number)
	}
	fmt.Fprintf(d.out, "#%d %s\nbranch: %s at %s\n%s\n", pr.Number, pr.Title, pr.Branch, pr.Head, pr.URL)
	head, headErr := d.run(root, "git", "rev-parse", "HEAD")
	dirty, dirtyErr := d.run(root, "git", "status", "--porcelain")
	if headErr != nil || dirtyErr != nil || strings.TrimSpace(string(head)) == "" {
		return d.refuse("cannot read local checkout state")
	}
	d.localState(pr, branch, strings.TrimSpace(string(head)), strings.TrimRight(string(dirty), "\r\n"))
	failed, problem := d.readChecks(root, number)
	if problem != "" {
		return d.refuse(problem)
	}
	if len(failed) == 0 {
		return d.displayChecks(root, number)
	}
	d.failures(failed, root, logLimit)
	return 1
}

func (d diagnosis) checkout() (string, string, string) {
	rootBytes, err := d.run("", "git", "rev-parse", "--show-toplevel")
	if err != nil || strings.TrimSpace(string(rootBytes)) == "" {
		return "", "", "cannot find repository root"
	}
	root := strings.TrimSuffix(string(rootBytes), "\n")
	if _, authErr := d.run(root, "gh", "auth", "status"); authErr != nil {
		if missing, ok := errors.AsType[*exec.Error](authErr); ok && missing != nil {
			return "", "", "gh is not installed"
		}
		return "", "", "gh is not authenticated"
	}
	branch, err := d.run(root, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || strings.TrimSpace(string(branch)) == "" {
		return "", "", "cannot read checkout branch"
	}
	return root, strings.TrimSpace(string(branch)), ""
}

func (d diagnosis) target(root, branch string, args []string) (string, string) {
	number := ""
	if len(args) != 0 {
		number = args[0]
	}
	if number == "" {
		data, err := d.run(root, "gh", "pr", "list", "--head", branch, "--state", "open", "--json", "number")
		var prs []struct {
			Number int `json:"number"`
		}
		if err != nil || json.Unmarshal(data, &prs) != nil || len(prs) == 0 || prs[0].Number <= 0 {
			return "", fmt.Sprintf("no open pull request for %s. Pass a number explicitly.", branch)
		}
		number = strconv.Itoa(prs[0].Number)
	}
	if n, err := strconv.Atoi(number); err != nil || n <= 0 {
		return "", "invalid pull request number"
	}
	return number, ""
}

func (d diagnosis) readChecks(root, number string) ([]diagnosisCheck, string) {
	data, err := d.run(root, "gh", "pr", "checks", number, "--json", "name,state,link")
	var checks []diagnosisCheck
	unreadable := fmt.Sprintf("cannot read checks for #%s", number)
	if json.Unmarshal(data, &checks) != nil || len(checks) == 0 {
		return nil, unreadable
	}
	failed := make([]diagnosisCheck, 0)
	for _, check := range checks {
		if check.Name == "" || check.State == "" {
			return nil, unreadable
		}
		if diagnosisFailure(check.State) {
			failed = append(failed, check)
		}
	}
	// Only usable known failures from a check-status exit are diagnostic
	// evidence. Partial stdout from a failed read leaves diagnosis inconclusive.
	if err != nil && (len(failed) == 0 || !diagnosisCheckStatus(err)) {
		return nil, unreadable
	}
	return failed, ""
}

func (d diagnosis) displayChecks(root, number string) int {
	display, err := d.run(root, "gh", "pr", "checks", number)
	if (err != nil && !diagnosisCheckStatus(err)) || len(display) == 0 {
		return d.refuse("cannot display checks for #" + number)
	}
	fmt.Fprintf(d.out, "\n== checks ==\nNothing is failing on #%s.\n", number)
	fmt.Fprint(d.out, string(display))
	return 0
}

func (pr diagnosisPR) valid() bool {
	return pr.Number > 0 && pr.Title != "" && pr.Branch != "" && pr.Head != "" && pr.URL != ""
}

func diagnosisCheckStatus(err error) bool {
	var status interface{ ExitCode() int }
	return errors.As(err, &status) && (status.ExitCode() == 1 || status.ExitCode() == 8)
}

func diagnosisFailure(state string) bool {
	switch state {
	case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED":
		return true
	}
	return false
}

func (d diagnosis) localState(pr diagnosisPR, branch, head, dirty string) {
	fmt.Fprintf(d.out, "\n== local state ==\nchecked out: %s at %.40s\n", branch, head)
	switch {
	case branch != pr.Branch:
		fmt.Fprintf(
			d.out,
			"MISMATCH: the pull request is on %s, this worktree is on %s.\nCheck out that branch before fixing anything.\n",
			pr.Branch,
			branch,
		)
	case head != pr.Head:
		fmt.Fprintln(
			d.out,
			"MISMATCH: local HEAD differs from the commit CI tested.\nThe logs below may describe code this worktree no longer has.",
		)
	default:
		fmt.Fprintln(d.out, "in sync with the commit CI tested")
	}
	if dirty != "" {
		fmt.Fprintf(d.out, "\nuncommitted changes present:\n%s\n", dirty)
	}
}

var diagnosisRunID = regexp.MustCompile(`/runs/([0-9]+)`)

func (d diagnosis) failures(failed []diagnosisCheck, root, logLimit string) {
	limit := 80
	if n, err := strconv.Atoi(logLimit); err == nil && n >= 0 {
		limit = n
	}
	fmt.Fprintln(d.out, "\n== failures ==")
	seen := make(map[string]bool)
	for _, check := range failed {
		fmt.Fprintf(
			d.out,
			"\n--------------------------------------------------------------\njob:        %s\nreproduce:  %s\nrun:        %s\n",
			check.Name,
			diagnosisReproducer(check.Name),
			check.Link,
		)
		match := diagnosisRunID.FindStringSubmatch(check.Link)
		if match == nil {
			fmt.Fprintln(d.out, "log:        (not a workflow run; open the link)")
			continue
		}
		id := match[1]
		if seen[id] {
			fmt.Fprintf(d.out, "log:        already printed above for run %s\n", id)
			continue
		}
		seen[id] = true
		log, err := d.run(root, "gh", "run", "view", id, "--log-failed")
		text := strings.TrimRight(string(log), "\r\n")
		if err != nil || text == "" {
			fmt.Fprintln(d.out, "log:        empty. The job may have been cancelled before it ran.")
			continue
		}
		lines := strings.Split(text, "\n")
		if len(lines) > limit {
			lines = lines[len(lines)-limit:]
		}
		fmt.Fprintf(d.out, "\nlast %d lines of the failing steps:\n", limit)
		if len(lines) > 0 {
			fmt.Fprintln(d.out, strings.Join(lines, "\n"))
		}
	}
	fmt.Fprintln(
		d.out,
		"\n--------------------------------------------------------------\nRun the reproducers above before changing anything. A failure that\ndoes not reproduce locally is a different problem from one that does.",
	)
}

func diagnosisReproducer(name string) string {
	// LC_ALL=C in the frozen shell folds only A through Z.
	name = strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, name)
	for _, mapping := range []struct{ patterns, task string }{
		{"prose|vale", "mise run lint-prose"},
		{"spell", "mise run devtools:lint-spelling"},
		{"markdown|rumdl", "mise run devtools:lint-markdown"},
		{"shell", "mise run devtools:lint-shell && mise run devtools:lint-shell-fmt"},
		{"yaml", "mise run devtools:lint-yaml"},
		{"toml", "mise run devtools:lint-toml"},
		{"workflow|actionlint", "mise run devtools:lint-workflows"},
		{"editorconfig", "mise run lint-editorconfig"},
		{"arch", "mise run devtools:lint-go-arch"},
		{"deadcode", "mise run devtools:lint-go-deadcode"},
		{"lint", "mise run lint"},
		{"race", "mise run devtools:test-go-race"},
		{"cover", "mise run devtools:cover-go-check"},
		{"test", "mise run test-full"},
		{"vendor", "mise run devtools:vendor-check"},
		{"vuln", "mise run devtools:vuln"},
		{"gitleaks|secret", "mise run devtools:gitleaks"},
		{"apm|validate", "apm install --frozen && apm audit --ci"},
		{"fuzz", "mise run fuzz"},
	} {
		for pattern := range strings.SplitSeq(mapping.patterns, "|") {
			if strings.Contains(name, pattern) {
				return mapping.task
			}
		}
	}
	return "(no local task maps to this job; read the log)"
}
