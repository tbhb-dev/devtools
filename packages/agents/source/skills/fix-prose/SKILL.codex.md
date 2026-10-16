---
name: fix-prose
description: >-
  Clear vale and cspell findings on a settled document through an isolated writer and an independent reviewer. Use when a draft's prose gates are red or repeated lint findings would pollute the parent context. Takes a target and its complete lint command.
---

# Reword a settled draft until its prose lint passes

Keep findings and retries out of the parent context. Findings, retries, and rejected wordings stay in the delegated agents, and the parent sees only verdicts. Use the skill once the document says what it should say. The writer changes wording rather than claims.

## Preflight

Parse the request into a target and a complete lint command, then run this from the repository holding the target:

```text
bash .agents/skills/fix-prose/scripts/preflight.sh <target> <lint command>
```

Stop on `BLOCKED` and say which condition applies. The preflight prints counts and verdicts rather than findings, on purpose.

@include parts/arguments.md

## The checklist

Where the harness provides `update_plan`, track these with it, keeping exactly one item `in_progress`. Otherwise work the list in order as written.

1. Record the baseline
2. Fix with the writer
3. Check for silenced checkers
4. Review independently
5. Lint and clean up

## Step 1: record the baseline

```text
bash .agents/skills/fix-prose/scripts/check-suppressions.sh --baseline <target>
```

It prints a `state:` path, a directory inside the worktree that git ignores. Save the absolute path. It identifies this workflow and records its target, and until cleanup the script refuses a second workflow on the same target. Pass the state to every delegated writer and reviewer in this run.

## Step 2: fix

Delegate `write-prose-fix` with `fork_turns: "none"`. Give it only the repository root, target, workflow state, lint command, and an instruction to follow that skill. Wait for its terminal report.

Where you have findings or a decision to hand over, write them to the notes file in the state directory before delegating:

```text
printf '%s\n' "<the reviewer's findings or the decision, verbatim>" > "<state>/notes"
```

The writer's context prints that file and works from it. The context script falls back to the older location, `fix-prose.notes` under the git directory, when the run has no notes.

Never edit the target in the parent. It returns `PROSE: CLEAN`, `PROSE: PARTIAL` with a short unresolved list, or `PROSE: BLOCKED`.

## Step 3: check for silenced checkers

```text
bash .agents/skills/fix-prose/scripts/check-suppressions.sh --verify <state> <target>
```

Silence means the writer edited nothing but the target. Any output identifies a file it shouldn't have changed. This runs in the parent because an agent certifying its own work certifies nothing. A finding means reverting that edit and taking the original lint finding back as unresolved.

## Step 4: review, and loop

Delegate `review-prose-fix` with `fork_turns: "none"`, and give it the repository root, target, and workflow state. It runs as an independent agent that didn't see the findings or the edits, and it reads the before and after against each other.

@include parts/after.md
