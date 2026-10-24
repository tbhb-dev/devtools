---
name: merge-pr
license: Apache-2.0
description: >-
  Squash merge a pull request. The commit message comes from this workflow rather than from GitHub, whose own version concatenates every commit on the branch into text no linter ever reads. A briefing script prints the published description, every commit the squash collapses, and the diffstat, then leaves a SQUASH_AGENTMSG skeleton whose body the caller writes. The commit-msg gates, an independent review, and an operator confirmation all run before the merge. Use this whenever the user asks to merge or land a pull request, including one nobody here authored such as a dependency bump.
hooks:
  PreToolUse:
    - matcher: Bash
      hooks:
        - type: command
          command: "${CLAUDE_PROJECT_DIR}/.claude/skills/merge-pr/scripts/guard-merge.sh"
---

# Merge a pull request

Use `gh-post` for issue and PR content, comments, reviews, comment edits, and release notes. Supply body text through `--body-file` or stdin. If `gh-post` is missing, stop and report the missing command. Never fall back to a direct GitHub content write.

Work the steps in order. Scripts enforce every gate here. Step 5's review script stamps what an independent reviewer cleared, bound to the pull request's head and base. Step 7's merge script refuses a message the stamp doesn't cover, a pull request that moved or isn't ready, and a merge nobody confirmed, then merges pinned to the reviewed head. Both hold in any session, including a subagent's.

A guard hook runs alongside them as a rail, refusing a direct `gh pr merge` and every other `gh pr` verb that changes a pull request. Reading remains permitted.

@include parts/why.md

## Preflight

!`bash ${CLAUDE_SKILL_DIR}/scripts/preflight.sh ${ARGUMENTS}`

## Step 0: the checklist

Track these steps with the session's task-list tools where the session provides them. Newer harnesses leave those tools out by default, and a session without them works the list in order as written.

@include parts/checklist.md

@include parts/mergeable.md

## Step 2: settle the description first

The description is on GitHub at this point, and often nothing sits on disk. Fetch it back before revising it:

```text
bash .claude/skills/merge-pr/scripts/populate-description.sh <number>
```

@include parts/description.md

Where it no longer describes the branch, revise `PR_AGENTDESC.md` in this session or use the optional `write-pr-description` helper, then run the `pr` review script, then republish with `bash .claude/skills/pr/scripts/create-pr.sh`.

## Step 3: write the squash message

```text
bash .claude/skills/merge-pr/scripts/squash-message.sh <number>
```

@include parts/squash-message.md

## Step 4: run the commit-msg gates

Fix the prose findings with the `fix-prose` skill first, passing the draft and the task that judges it:

```text
Skill(fix-prose, args: "SQUASH_AGENTMSG bash .claude/skills/merge-pr/scripts/lint-squash-msg.sh <number>")
```

Then run the wrapper yourself:

```text
bash .claude/skills/merge-pr/scripts/lint-squash-msg.sh <number>
```

@include parts/gates.md

## Step 5: review the message

```text
bash .claude/skills/merge-pr/scripts/review.sh <number>
```

In auto mode, the operator can add `Bash(bash .claude/skills/merge-pr/scripts/review.sh *)` to `permissions.allow` in their Claude Code settings after agreeing to the review. The unattended wrapper is `Bash(bash .claude/skills/agent-merge-pr/scripts/review.sh *)`. Both current scripts accept a pull request number and `--note`, but no `--unblock` flag. An owner adds these permissions, never the agent.

Pass `--note <file>` to include a decision the operator actually made, with the operator's reason. Never use it for the agent's own preference.

@include parts/review.md

## Step 6: confirm with the operator

`AskUserQuestion` truncates its options. The operator has to read the message in your message text rather than in the widget. Print this first, verbatim:

```text
Merging: #<number> <title>
Into:    <base branch>
Commits: <count> collapsing into one

<the entire SQUASH_AGENTMSG contents, verbatim>
```

@include parts/confirm.md

Where preflight reported `merge: GRANTED` or a delegation check, print the preceding block so the operator still sees the message, state which one this merge goes under, and move to step 7 without calling `AskUserQuestion`.

Otherwise call `AskUserQuestion` with `question` set to `Squash merge with this message?`, `header` set to `Merge`, `multiSelect` set to `false`, and these four options in order:

| Label | Description |
| --- | --- |
| `Merge it` | The subject line, verbatim |
| `Revise the message` | `Redraft the message, review it again, and come back.` |
| `Wait on the checks` | `Hand this to watch-pr first, and merge once it reports green.` |
| `Stop here` | `Leave the pull request, the draft, and the branch untouched.` |

Follow the answer. `Merge it` is what `--confirmed` stands for in step 7. Revising sends you back through the review, because the stamp binds bytes.

## Step 7: merge and clean up

```text
bash .claude/skills/merge-pr/scripts/squash-merge.sh [--confirmed] <number>
```

Pass `--confirmed` only when the operator chose `Merge it` in step 6. Merge only through this script. The guard hook refuses a `gh pr merge` you write out yourself, because a direct call skips every gate below and nothing lints a squash commit afterwards.

@include parts/merge.md

## API quota

Identity and explicit readers use GraphQL, which doesn't spend REST core quota. Each checks snapshot uses one GraphQL call. Quota failures return 76. The printed message says which limit applied and what to do next: wait at least a minute after a secondary limit, or read the budget with a GraphQL `rateLimit { remaining resetAt }` query once a primary GraphQL limit has passed. GitHub doesn't report the reset time with the failure. No script looks it up through `/rate_limit`. The original error follows unchanged. Other API failures keep GitHub's error. The watcher returns 2 for ordinary check-read failures. GitHub CLI writes can still spend REST quota resolving or applying labels, milestones, reviewers, and assignees. Scripts never retry writes automatically.
