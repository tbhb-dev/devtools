---
name: watch-pr
description: >-
  Wait for a pull request's checks to settle in one long-running command, then report passed, skipped, and failed jobs with run links. Use whenever the user asks to watch a pull request or wait for CI, and whenever another workflow needs the result before continuing.
---

# Watch a pull request

Use `gh-post` for issue and PR content, comments, reviews, comment edits, and release notes. Supply body text through `--body-file` or stdin. If `gh-post` is missing, stop and report the missing command. Never fall back to a direct GitHub content write.

Wait for the checks on a pull request to settle, then say what happened. This skill ends when the checks stop moving. Fixing what failed belongs to `fix-pr`, and merging belongs to `merge-pr`.

## Step 1: watch

Use the pull request number in the task, or omit it for the current branch's open pull request. Start one blocking command:

```bash
bash .agents/skills/watch-pr/scripts/watch-checks.sh <number>
```

Set the execution yield to at most 30 seconds. If it returns a running session, continue it with `write_stdin` using empty input and a wait of up to 300 seconds. Keep waiting on that same session. Don't start another watcher or poll `gh pr checks` between waits. Send a concise progress update at least once a minute while checks remain pending.

The lines to expect:

- `WATCHING #<number>` once, at the start
- `PASS <job>`, `SKIP <job>`, or `FAIL <job> <run link>`, one per check as it settles
- `ALL GREEN #<number>`, `FAILED #<number> with N failing check(s)`, `TIMEOUT #<number>`, or `NO CHECKS #<number>` to close
- `ERROR <reason>` where `gh` failed or answered with something other than a list of checks

Each poll reads the checks through one `gh` call. A rerun replaces the run it repeats, so an old failure that a rerun fixed doesn't report, and two workflows sharing a job name stay apart.

The exit code reports the outcome: `0` all passed, `1` something failed, `2` inconclusive when the wait expires or no check registers. A `gh` failure also returns `2`.

`PR_CHECKS_TIMEOUT` sets the whole wait and `PR_CHECKS_INTERVAL` the poll spacing, both in seconds.

## Step 2: report

Say which checks failed and hand back. Give the pull request, the failing jobs, and the run links the script printed. Invoke `fix-pr` with the same number only when the caller asked for a fix.

A timeout isn't a failure, and neither is a pull request with no checks. Say the checks were still running, or that none registered, give the bound, and let the caller decide whether to wait again. An empty check set never counts as green.

## API quota

Identity and explicit readers use GraphQL, which doesn't spend REST core quota. Each checks snapshot uses one GraphQL call. Quota failures return 76. The printed message says which limit applied and what to do next: wait at least a minute after a secondary limit, or read the budget with a GraphQL `rateLimit { remaining resetAt }` query once a primary GraphQL limit has passed. GitHub doesn't report the reset time with the failure. No script looks it up through `/rate_limit`. The original error follows unchanged. Other API failures keep GitHub's error. The watcher returns 2 for ordinary check-read failures. GitHub CLI writes can still spend REST quota resolving or applying labels, milestones, reviewers, and assignees. Scripts never retry writes automatically.
