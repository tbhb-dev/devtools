---
name: resolve-rebase-conflicts
license: Apache-2.0
description: >-
  Settle the conflicts a stopped rebase left behind. Mechanical ones resolve without asking and the rest arrive with the evidence in front of you. It classifies each conflicted path first. An append-only sorted list becomes the verified union of both sides. A generated file takes one side whole and waits for its generator. Everything else is a real disagreement with what each side changed printed beside it. The rebase skill calls this at every stop. Use it directly whenever a rebase or cherry-pick stops on a conflict and the user asks to resolve it.
hooks:
  PreToolUse:
    - matcher: Bash
      hooks:
        - type: command
          command: "${CLAUDE_PROJECT_DIR}/.claude/skills/resolve-rebase-conflicts/scripts/guard-resolve.sh"
---

# Resolve rebase conflicts

Settle every conflicted path and stage it, so the rebase can advance. This skill ends there. Continuing the replay and verifying the result belong to the `rebase` skill.

A guard hook runs alongside. It refuses `--ours` and `--theirs`, which mean the opposite of what they read during a rebase, and it refuses staging a file with markers still in it. It also refuses whole-tree staging.

@include parts/inversion.md

## Step 0: the checklist

Track these steps with the session's task-list tools where the harness provides them. Newer harnesses leave those tools out by default, and a session without them works the list in order as written.

@include parts/checklist.md

## Step 1: classify before editing anything

```text
bash .claude/skills/resolve-rebase-conflicts/scripts/classify-conflicts.sh
```

@include parts/classify.md

## Step 2: resolve the mechanical ones

```text
bash .claude/skills/resolve-rebase-conflicts/scripts/resolve-union.sh <path> [path ...]
```

@include parts/union.md

## Step 3: take a whole side where that's the answer

```text
bash .claude/skills/resolve-rebase-conflicts/scripts/take-side.sh <path> base-side
bash .claude/skills/resolve-rebase-conflicts/scripts/take-side.sh <path> replayed-side
```

@include parts/take-side.md

## Step 4: read the rest

@include parts/read-rest.md

The guard refuses both forms, along with any file with markers still in it.

## Step 5: hand back

@include parts/hand-back.md

@include parts/preconditions.md
