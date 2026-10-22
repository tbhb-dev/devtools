---
name: watch-pr
license: Apache-2.0
description: >-
  Wait for a pull request's checks to settle, then report what passed and what failed. Blocks in one call rather than polling. A wait then costs one turn instead of one per look. Use this whenever the user asks to watch a pull request or wait on CI, and whenever a workflow needs that answer before it can go on.
hooks:
  PreToolUse:
    - matcher: Bash
      hooks:
        - type: command
          command: "${CLAUDE_PROJECT_DIR}/.claude/skills/watch-pr/scripts/guard-watch.sh"
---

# Watch a pull request

Use `gh-post` for issue and PR content, comments, reviews, comment edits, and release notes. Supply body text through `--body-file` or stdin. If `gh-post` is missing, stop and report the missing command. Never fall back to a direct GitHub content write.

Wait for the checks on a pull request to settle, then say what happened. This skill ends when the checks stop moving. Fixing what failed belongs to `fix-pr`, and merging belongs to `merge-pr`.

A guard hook runs alongside and refuses the watching forms this skill wraps, so a hand-rolled poll can't quietly replace the watcher. A plain `gh pr checks` for a one-off look remains allowed.

## Which pull request

`$ARGUMENTS` names the pull request number when the caller knows it. An empty value means whatever is open for the current branch.

## Step 1: watch

Arm the watcher through the `Monitor` tool, which turns each line the script prints into a notification:

```text
Monitor({
  command: "bash .claude/skills/watch-pr/scripts/watch-checks.sh <number>",
  description: "checks on pull request <number>",
  timeout_ms: 1800000,
  persistent: false,
})
```

Omit the number to use the current branch's open pull request.

`Monitor` is the right tool rather than a blocking call, because the script emits one line per check the moment that check settles. A failure arrives while the slower jobs are still going. The diagnosis then starts earlier and no turn goes to waiting. The script exits once nothing is pending, which ends the watch by itself.

Never poll `gh pr checks` in a loop of your own. Each look costs a turn and reports a state that has already moved on.

The lines to expect:

- `WATCHING #<number>` once, at the start
- `PASS <job>`, `SKIP <job>`, or `FAIL <job> <run link>`, one per check as it settles
- `ALL GREEN #<number>`, `FAILED #<number> with N failing check(s)`, `TIMEOUT #<number>`, or `NO CHECKS #<number>` to close
- `ERROR <reason>` where `gh` failed or answered with something other than a list of checks

Each poll reads the checks through one `gh` call.

A rerun replaces the run it repeats, so an old failure that a rerun fixed doesn't report, and two workflows sharing a job name stay apart.

Running the same command through `Bash` also works and blocks until the same ending, with the exit code reporting the outcome: `0` all passed, `1` something failed, `2` inconclusive when the wait expires or no check registers. A `gh` failure also returns `2`. Prefer `Monitor`. Use the blocking form only where the next step can't start until the answer arrives.

Override the bounds through the environment when a run is unusually slow. `PR_CHECKS_TIMEOUT` sets the whole wait and `PR_CHECKS_INTERVAL` the poll spacing, both in seconds. Keep `timeout_ms` past `PR_CHECKS_TIMEOUT` so the script reports its own timeout rather than dying at the tool's.

## Step 2: report

Say which checks failed and hand back. Give the pull request, the failing jobs, and the run links the script printed.

Where the caller asked only to watch, stop here. Where the caller asked for a fix, invoke `fix-pr` with the same number. That skill reads the logs and identifies the local task reproducing each failure.

A timeout isn't a failure, and neither is a pull request with no checks. Say the checks were still running, or that none registered, give the bound, and let the caller decide whether to wait again. An empty check set never counts as green.

## Preconditions

- `gh` installed and authenticated
- a pull request open for the branch, or a number in `$ARGUMENTS`

The script checks both and stops with a plain message when either is missing.

## API quota

Identity and explicit readers use GraphQL, which doesn't spend REST core quota. Each checks snapshot uses one GraphQL call. Quota failures return 76 and print the resource's reset time. Other API failures keep GitHub's error. The watcher returns 2 for ordinary check-read failures. A missing reset header needs one REST `/rate_limit` lookup, which doesn't consume core quota. GitHub CLI writes can still spend REST quota resolving or applying labels, milestones, reviewers, and assignees. Scripts never retry writes automatically.
