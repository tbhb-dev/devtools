---
name: commit
license: Apache-2.0
description: >-
  Group changes into one atomic commit, draft a Conventional Commit message in COMMIT_AGENTMSG, put it through an independent review and the commit-msg gates, confirm it with the operator, then commit and rebase. Use this skill whenever the user asks to commit work in a tbhb repo ("commit this," "commit the staged changes," "write a commit message") or whenever a task ends in creating a commit.
hooks:
  PreToolUse:
    - matcher: Bash
      hooks:
        - type: command
          command: "${CLAUDE_PROJECT_DIR}/.claude/skills/commit/scripts/guard-git.sh"
---

# Commit workflow

Work the steps in order. Scripts enforce every gate here. Step 6's review script stamps what an independent reviewer cleared. Step 9's commit script refuses an inline `-m` message or a `--no-verify`, along with a draft or index the stamp doesn't cover and a commit nobody confirmed. Both hold in any session, including a subagent's.

A guard hook runs alongside them as a rail, refusing whole-tree staging and any `git commit` you write out yourself. It stays registered for the rest of the session, so it applies only while `HEAD` sits on the commit preflight recorded. Once step 9 moves `HEAD` past that mark, the guard refuses nothing. A second commit means invoking this skill again rather than carrying on from here.

## Preflight

!`bash ${CLAUDE_SKILL_DIR}/scripts/preflight.sh`

## Step 0: the checklist

Track these steps with the session's task-list tools where it provides them. Newer harnesses leave those tools out by default, and a session without them works the list in order as written. They're the checklist the rest of this document expands.

1. Choose the rebase base
2. Group the changes into atomic commits
3. Stage the paths for this commit
4. Draft the message in COMMIT_AGENTMSG
5. Pass the prose gates with fix-prose
6. Review the draft with the review script
7. Run the commit-msg gates
8. Confirm the message with the operator
9. Commit and rebase

Stop before any of it if preflight reports a rebase, merge, or cherry-pick in progress, or a missing precondition. Say what's wrong and hand back.

@include parts/base.md

Worktrees under `.claude/worktrees` are the usual layout here, and each one has its own checkout of the branch.

@include parts/group-stage-draft.md

## Step 5: pass the prose gates

Invoke the `fix-prose` skill before the review, passing the draft and the task that judges it:

```text
Skill(fix-prose, args: "COMMIT_AGENTMSG mise run lint-commit-msg")
```

It runs the lint rounds in a subagent, so the findings and the retries stay out of this session. The commit scope is stricter than the repository-wide one, and a message copied verbatim out of the diff still fails it, so this is rarely a no-op.

Order matters here. The review in the next step stamps the exact bytes it read, and a later lint fix voids that stamp and costs another review round over a comma. Running fix-prose first holds the loop to one round.

Where it returns `PROSE: PARTIAL`, a finding needs a decision rather than another round. Read the short list, settle it, and send the answer back in the arguments.

## Step 6: review the draft

```text
bash .claude/skills/commit/scripts/review.sh
```

Add `--amend` when step 9 is going to amend.

Pass `--note <file>` to include a decision the operator actually made, with the operator's reason. Never use it for the agent's own preference.

@include parts/review.md

@include parts/gates.md

## Step 8: confirm with the operator

`AskUserQuestion` truncates its options. The operator reads the message in your message text rather than in the widget. Print this first, verbatim:

```text
Staged: <paths, comma separated>

<the entire COMMIT_AGENTMSG contents, verbatim>
```

@include parts/confirm.md

Where preflight reported `commit: GRANTED`, print the preceding block so the operator still sees the message, state the grant this commit goes under, and move to step 9 without calling `AskUserQuestion`. Where it reported a delegation check, do the same, and the script consults that check at commit time.

Otherwise call `AskUserQuestion` with `question` set to `Commit this message?`, `header` set to `Commit`, `multiSelect` set to `false`, and these four options in order:

| Label | Description |
| --- | --- |
| `Commit it` | The subject line, verbatim |
| `Revise the message` | `Staging stands. Redraft the message, review it again, and come back.` |
| `Restage and redraft` | `The grouping is wrong. Regroup the changes and start from step 2.` |
| `Stop here` | `Leave the draft, the index, and the branch untouched.` |

Follow the answer. `Commit it` is what `--confirmed` stands for in step 9. Revising or restaging sends you back through the review, because the stamp binds bytes.

## Step 9: commit and rebase

```text
bash .claude/skills/commit/scripts/commit.sh [--confirmed]
```

Pass `--confirmed` only when the operator chose `Commit it` in step 8. Commit only through this script. The guard hook refuses a `git commit` you write out yourself whatever flags it passes, because the script runs every gate named below and a hook on the tool call can't inspect a script's contents.

@include parts/commit.md
