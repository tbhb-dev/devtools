---
name: rebase
description: >-
  Replay the current branch onto its base and establish that the result is sound. Preflight settles the base by comparing the local default branch against origin's copy. The rebase runs under pinned settings. A range-diff afterwards says what the replay did to each commit. The gates then run against a tree no commit ever saw. Conflicts route to the resolve-rebase-conflicts skill. Use this whenever the user asks to rebase or to bring a branch current, and whenever a workflow needs the branch moved first.
---

# Rebase workflow

Move the branch onto its base, then establish that what came out is right.

Codex doesn't provide a skill-scoped command hook. Use only the scripts named here to pin the rebase settings and check conflict markers before continuing. The continuation script also proves an empty skip. Never use a bare stash pop, start the rebase by hand, or force-push without a lease.

## Preflight

```text
bash .agents/skills/rebase/scripts/preflight.sh
```

## Step 0: the checklist

Where the harness provides `update_plan`, track these with it, keeping exactly one item `in_progress` at a time. Otherwise work the list in order as written.

@include parts/checklist.md

@include parts/base-and-tree.md

## Step 3: start the rebase

```text
bash .agents/skills/rebase/scripts/start-rebase.sh <base>
```

@include parts/start.md

## Step 4: resolve what it stops on

```text
bash .agents/skills/rebase/scripts/rebase-status.sh
```

@include parts/resolve.md

```text
bash .agents/skills/rebase/scripts/continue-rebase.sh
```

@include parts/resolve-after.md

## Step 5: verify the result

```text
bash .agents/skills/rebase/scripts/verify-rebase.sh
```

@include parts/verify.md
