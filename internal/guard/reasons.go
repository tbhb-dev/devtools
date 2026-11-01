// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package guard

// Keep the shell guards' reason bytes intact for the later caller flip.
const readingNote = "\n\nReading stays open: gh pr list, view, diff, status, and checks all run\nwithout asking."

const publishReason = "`gh pr %s` changes the pull request, and this workflow keeps every\n" + `such change behind one entry point:

` + "  " + `bash .claude/skills/pr/scripts/create-pr.sh

That script validates the description against the template, checks that
the review stamp covers the exact bytes on disk and the branch as it
stands, resolves the labels and every issue reference before anything is
published, and updates the open pull request when one already exists.` + readingNote

const mergeReason = "`gh pr %s` changes the pull request, and merging runs through one\n" + `entry point:

` + "  " + `bash .claude/skills/merge-pr/scripts/squash-merge.sh [--confirmed] <number>

That script checks that the message names this pull request, that the
independent review cleared the exact bytes on disk at the head about to
merge, that the message passes the commit-msg gates, that the pull
request is open, ready, mergeable, and green, and that somebody
confirmed the merge. It then merges pinned to the reviewed head,
removes the drafts, and deletes the branch.

Calling gh directly writes a message onto the default branch that
nothing reviewed, and nothing lints a squash commit afterwards.` + readingNote

const watchChecksReason = `This skill wraps the watch so each check becomes one event rather than
one turn:

` + "  " + `Monitor({
` + "    " + `command: "bash .claude/skills/watch-pr/scripts/watch-checks.sh <number>",
` + "    " + `description: "checks on pull request <number>",
` + "    " + `timeout_ms: 1800000,
` + "    " + `persistent: false,
` + "  " + `})

The script emits PASS, SKIP, and FAIL lines as they settle, then exits.
A single 'gh pr checks' without --watch is fine for a one-off look.`

const watchRunReason = `Watching a run directly reports a workflow rather than the pull request,
and it blocks a turn to do it. Use the skill's watcher instead:

` + "  " + `bash .claude/skills/watch-pr/scripts/watch-checks.sh <number>`

const diagnoseReason = `Sweeping the failing steps is what the skill's own diagnosis does, in one
call, with the reproducing task named beside each failure:

` + "  " + `bash .claude/skills/fix-pr/scripts/diagnose.sh <number>

Run that first. Once you need more than the tail it printed, a single
job's full log stays available:

` + "  " + `gh run view <run-id> --job <job-id> --log`

const forceReason = `A bare force push overwrites whatever the remote has, including commits
this worktree never saw. Use the lease instead:

` + "  " + `git push --force-with-lease origin HEAD

It refuses when the remote moved since your last fetch, which is exactly
the case a bare --force would destroy.`

const fixWatchReason = `Waiting on checks belongs to the watch-pr skill, which streams each result
as an event rather than blocking a turn. Invoke that skill instead.`
