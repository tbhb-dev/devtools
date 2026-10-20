---
name: merge-pr
description: >-
  Squash merge a pull request. The commit message comes from this workflow rather than from GitHub, whose own version concatenates every commit on the branch into text no linter ever reads. A briefing script prints the published description, every commit the squash collapses, and the diffstat, then leaves a SQUASH_AGENTMSG skeleton whose body the caller writes. The commit-msg gates, an independent review, and an operator confirmation all run before the protected merge script. Use this whenever the user asks to merge or land a pull request, including one nobody here authored such as a dependency bump.
---

# Merge a pull request

Work the steps in order. Scripts enforce every gate here. Step 5's review script stamps what an independent reviewer cleared, bound to the pull request's head and base. Step 7's merge script refuses a message the stamp doesn't cover, a pull request that moved or isn't ready, and a merge nobody confirmed, then merges pinned to the reviewed head. Codex doesn't provide a skill-scoped command hook, so those scripts are the whole enforcement. Never run `gh pr merge` yourself.

@include parts/why.md

## Preflight

Run this from the repository before anything else, passing the pull request number when the request specifies one:

```text
bash .agents/skills/merge-pr/scripts/preflight.sh [number]
```

## Step 0: the checklist

Where the harness provides `update_plan`, track these with it, keeping exactly one item `in_progress` at a time. Otherwise work the list in order as written.

@include parts/checklist.md

@include parts/mergeable.md

## Step 2: settle the description first

The description is on GitHub at this point, and often nothing sits on disk. Fetch it back before revising it:

```text
bash .agents/skills/merge-pr/scripts/populate-description.sh <number>
```

@include parts/description.md

Where it no longer describes the branch, revise `PR_AGENTDESC.md` in this session or use the optional `write-pr-description` helper, then run the `pr` review script, following its review loop, then republish with `bash .agents/skills/pr/scripts/create-pr.sh`.

## Step 3: write the squash message

```text
bash .agents/skills/merge-pr/scripts/squash-message.sh <number>
```

@include parts/squash-message.md

## Step 4: run the commit-msg gates

Fix the prose findings with the `fix-prose` skill first, on `SQUASH_AGENTMSG bash .agents/skills/merge-pr/scripts/lint-squash-msg.sh <number>`. Then run the wrapper yourself:

```text
bash .agents/skills/merge-pr/scripts/lint-squash-msg.sh <number>
```

@include parts/gates.md

## Step 5: review the message

```text
bash .agents/skills/merge-pr/scripts/review.sh <number>
```

Don't delegate the review to an agent of your own. Don't write a verdict file. The reviewer is a `codex exec` child in a read-only sandbox and needs the network, so escalate the script's permissions where this session's sandbox blocks it.

Pass `--note <file>` to include a decision the operator actually made, with the operator's reason. Never use it for the agent's own preference.

@include parts/review.md

## Step 6: confirm with the operator

Show the operator the pull request and the complete message first:

```text
Merging: #<number> <title>
Into:    <base branch>
Commits: <count> collapsing into one

<the entire SQUASH_AGENTMSG contents, verbatim>
```

@include parts/confirm.md

Where preflight reported `merge: GRANTED` or a delegation check, state which one this merge goes under and move to step 7 without `--confirmed`.

Otherwise end the turn and ask `Squash merge with this message?`, offering `Merge it`, `Revise the message`, `Wait on the checks`, and `Stop here`. Don't merge in the turn that asks. The original request to merge doesn't count as this confirmation. `Merge it` is what `--confirmed` stands for in step 7. Revising sends you back through the review, because the stamp binds bytes.

## Step 7: merge and clean up

```text
bash .agents/skills/merge-pr/scripts/squash-merge.sh [--confirmed] <number>
```

Pass `--confirmed` only when the operator answered `Merge it` in step 6. Merge only through this script.

@include parts/merge.md

## API quota

Identity and explicit readers use GraphQL, which doesn't spend REST core quota. Each checks snapshot uses one GraphQL call. Quota failures return 76 and print the resource's reset time. Other API failures keep GitHub's error. The watcher returns 2 for ordinary check-read failures. A missing reset header needs one REST `/rate_limit` lookup, which doesn't consume core quota. GitHub CLI writes can still spend REST quota resolving or applying labels, milestones, reviewers, and assignees. Scripts never retry writes automatically.
