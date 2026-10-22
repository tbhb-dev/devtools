---
name: commit
description: >-
  Group changes into one atomic commit, draft a Conventional Commit message in COMMIT_AGENTMSG, put it through an independent review and the commit-msg gates, confirm it with the operator, then commit through the protected wrapper and rebase. Use this skill whenever the user asks to commit work in a tbhb repository or whenever a task ends in creating a commit.
---

# Commit workflow

Work the steps in order. Scripts enforce every gate here. Step 6's review script stamps what an independent reviewer cleared. Step 9's commit script refuses an inline `-m` message or a `--no-verify`, along with a draft or index the stamp doesn't cover and a commit nobody confirmed. Codex doesn't provide a skill-scoped command hook, so those scripts are the whole enforcement. Never run `git commit` yourself and never bypass the hooks.

## Preflight

Run this from the repository holding the changes before anything else:

```text
bash .agents/skills/commit/scripts/preflight.sh
```

## Step 0: the checklist

Where the harness provides `update_plan`, track these with it, keeping exactly one item `in_progress` at a time. Otherwise work the list in order as written.

1. Choose the rebase base
2. Group the changes into atomic commits
3. Stage the paths for this commit
4. Draft the message in COMMIT_AGENTMSG
5. Pass the prose gates with fix-prose
6. Review the draft with the review script
7. Run the commit-msg gates
8. Confirm the message with the operator
9. Commit and rebase

Stop for a missing precondition or an operation in progress other than the base merge described below. Say what's wrong and hand back.

@include parts/base.md

@include parts/merge.md

@include parts/group-stage-draft.md

## Step 5: pass the prose gates

Invoke the `fix-prose` skill on `COMMIT_AGENTMSG mise run lint-commit-msg` before the review. The commit scope is stricter than the repository-wide one, so this is rarely a no-op.

Order matters here. The review in the next step stamps the exact bytes it read, and a later lint fix voids that stamp and costs another review round.

## Step 6: review the draft

```text
bash .agents/skills/commit/scripts/review.sh
```

Add `--amend` when step 9 is going to amend. Don't delegate the review to an agent of your own, and don't write a verdict file.

The reviewer is a `codex exec` child in a read-only sandbox and needs the network. Escalate the script's permissions where this session's sandbox blocks it.

Pass `--note <file>` to include a decision the operator actually made, with the operator's reason. Never use it for the agent's own preference.

@include parts/review.md

@include parts/gates.md

## Step 8: confirm with the operator

Show the operator the staged paths and the complete message first:

```text
Staged: <paths, comma separated>

<the entire COMMIT_AGENTMSG contents, verbatim>
```

@include parts/confirm.md

Where preflight reported `commit: GRANTED` or a delegation check, state which one this commit goes under and move to step 9 without `--confirmed`.

Otherwise end the turn and ask `Commit this message?`, offering `Commit it`, `Revise the message`, `Restage and redraft`, and `Stop here`. Don't commit in the turn that asks. The original request to commit doesn't count as this confirmation. `Commit it` is what `--confirmed` stands for in step 9. Revising or restaging sends you back through the review, because the stamp binds bytes.

## Step 9: commit and rebase

```text
bash .agents/skills/commit/scripts/commit.sh [--confirmed]
```

Pass `--confirmed` only when the operator answered `Commit it` in step 8. Commit only through this script.

@include parts/commit.md
