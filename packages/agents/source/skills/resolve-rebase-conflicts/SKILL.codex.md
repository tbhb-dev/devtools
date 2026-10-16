---
name: resolve-rebase-conflicts
description: >-
  Settle the conflicts a stopped rebase left behind. Mechanical ones resolve without asking and the rest arrive with the evidence in front of you. It classifies each conflicted path first. An append-only sorted list becomes the verified union of both sides. A generated file takes one side whole and waits for its generator. Everything else is a real disagreement with what each side changed printed beside it. The rebase skill calls this at every stop. Use it directly whenever a rebase or cherry-pick stops on a conflict and the user asks to resolve it.
---

# Resolve rebase conflicts

Settle every conflicted path and stage it, so the rebase can advance. This skill ends there. Continuing the replay and verifying the result belong to the `rebase` skill.

Codex doesn't provide a skill-scoped command hook. Never use `--ours` or `--theirs`. The scripts use unambiguous side names. Stage paths only by name, after checking each one for markers, and return through the continuation script, which checks the staged blobs again.

@include parts/inversion.md

## Step 0: the checklist

Where the harness provides `update_plan`, track these with it, keeping exactly one item `in_progress` at a time. Otherwise work the list in order as written.

@include parts/checklist.md

## Step 1: classify before editing anything

```text
bash .agents/skills/resolve-rebase-conflicts/scripts/classify-conflicts.sh
```

@include parts/classify.md

## Step 2: resolve the mechanical ones

```text
bash .agents/skills/resolve-rebase-conflicts/scripts/resolve-union.sh <path> [path ...]
```

@include parts/union.md

## Step 3: take a whole side where that's the answer

```text
bash .agents/skills/resolve-rebase-conflicts/scripts/take-side.sh <path> base-side
bash .agents/skills/resolve-rebase-conflicts/scripts/take-side.sh <path> replayed-side
```

@include parts/take-side.md

## Step 4: read the rest

@include parts/read-rest.md

## Step 5: hand back

@include parts/hand-back.md

@include parts/preconditions.md
