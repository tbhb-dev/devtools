// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"fmt"
	"io"
	"os/exec"
	"slices"
)

const usage = `Usage: gh-post <operation> [flags]
	gh-post --help
	gh-post --check-deps

Global flags:
	--help         Show all operations and flags without network access.
	--check-deps   Check git and gh on PATH without executing them.

Operations accept -R, --repo OWNER/REPO unless an exact form is shown.
Content operations accept --body-file PATH and default to stdin;
--body-file - also reads stdin. PR ready and close never read stdin.
An issue edit with --type reads no stdin unless --body-file is given.

Operations and additional flags:
	issue create    --title TEXT --label NAME --assignee LOGIN --milestone NAME
	issue comment <issue>  --edit-last --create-if-none
	issue close <issue>    --reason completed|"not planned" --body-file PATH
	issue reopen <issue>   --body-file PATH
	issue edit <issue>     --title TEXT --add-label NAME --remove-label NAME
	--type Feature|Defect|Chore|Spike
	--add-assignee LOGIN --remove-assignee LOGIN
	--milestone NAME --remove-milestone
	pr create       --title TEXT --base BRANCH --head BRANCH --label NAME
	--assignee LOGIN --reviewer LOGIN --milestone NAME --draft
	pr edit <pr>    --title TEXT --base BRANCH --add-label NAME --remove-label NAME
	--add-assignee LOGIN --remove-assignee LOGIN --add-reviewer LOGIN
	--remove-reviewer LOGIN --milestone NAME --remove-milestone
	issue label <issue>    -R OWNER/REPO --add change:NAME --remove change:NAME
	pr label <pr>   -R OWNER/REPO --add change:NAME --remove change:NAME
	pr ready <pr>   --head SHA --dry-run
	pr close <pr>   --head SHA --comment-file PATH --dry-run
	pr comment <pr> --edit-last --create-if-none
	pr review <pr>  --commit SHA (--approve | --comment | --request-changes)
	comment edit <comment> --kind issue|review
	review edit <pr> <review-id>
	release create <tag>   --title TEXT --target REF --draft --prerelease --verify-tag
	release edit <tag>     --title TEXT --draft --prerelease
	run rerun-failed <run-id> --repo OWNER/REPO

Run rerun-failed accepts exactly that form, with a positive decimal run ID,
and reads no stdin. Only completed failure, cancelled or timed_out runs
qualify. It reserves each run attempt in machine state before one failed-jobs
POST and audits the reservation. A repeated attempt is refused. After an
uncertain write it reads the run once, exits 2, and never retries the write.
It never reruns every job, cancels a run, or polls. Keep the state receipts.

New reviews require a 40-hex --commit matching the current PR head and
first body line Reviewed head: <sha> (the SHA may be wrapped in backticks).

PR ready and close require a full 40-hex --head, checked before the write
and at read-back. Ready requires an open draft; close requires an open PR
and a nonempty scanned reason file (at most 65536 characters).

Issue close and reopen require an explicit --body-file (use - for stdin).
Exit 0: posted and audited. Exit 1: scan, identity, or state refusal,
including an issue already in the requested state, or read-back mismatch.
Exit 2: invalid input, configuration, GitHub command, or audit failure.

Only these two PR verbs accept --dry-run; neither accepts --body-file.

Issue and PR label require -R, at least one --add or --remove (each
repeatable), and read no stdin. Only labels starting with change: are
accepted, and a comma or newline in a label is refused. A label the
repository lacks is refused rather than created. Neither verb accepts
--body-file.
`

func checkDeps(errOut io.Writer) int {
	code := 0
	for _, name := range []string{"git", "gh"} {
		if _, err := exec.LookPath(name); err != nil {
			fmt.Fprintf(errOut, "gh-post: missing dependency: %s\n", name)
			code = exitEnvironment
		}
	}
	return code
}

func informational(args []string, out, errOut io.Writer) (int, bool) {
	switch {
	case slices.Equal(args, []string{"--help"}):
		fmt.Fprint(out, usage)
		return 0, true
	case slices.Equal(args, []string{"--check-deps"}):
		return checkDeps(errOut), true
	default:
		return 0, false
	}
}
