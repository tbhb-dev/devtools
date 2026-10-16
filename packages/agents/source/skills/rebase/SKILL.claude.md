---
name: rebase
license: Apache-2.0
description: >-
  Replay the current branch onto its base and establish that the result is sound. Preflight settles the base by comparing the local default branch against origin's copy. The rebase runs under pinned settings. A range-diff afterwards says what the replay did to each commit. The gates then run against a tree no commit ever saw. Conflicts route to the resolve-rebase-conflicts skill. Use this whenever the user asks to rebase or to bring a branch current, and whenever a workflow needs the branch moved first.
hooks:
  PreToolUse:
    - matcher: Bash
      hooks:
        - type: command
          command: "${CLAUDE_PROJECT_DIR}/.claude/skills/rebase/scripts/guard-rebase.sh"
---

# Rebase workflow

Move the branch onto its base, then establish that what came out is right.

A guard hook runs alongside. It refuses a rebase started by hand and a continue past a conflict git never checked. It also refuses a dropped commit that isn't empty, a bare stash pop, and a force push without a lease.

## Preflight

!`bash ${CLAUDE_SKILL_DIR}/scripts/preflight.sh`

## Step 0: the checklist

Track these steps with the session's task-list tools where the harness provides them. Newer harnesses leave those tools out by default, and a session without them works the list in order as written.

@include parts/checklist.md

@include parts/base-and-tree.md

## Step 3: start the rebase

```text
bash .claude/skills/rebase/scripts/start-rebase.sh <base>
```

@include parts/start.md

## Step 4: resolve what it stops on

```text
bash .claude/skills/rebase/scripts/rebase-status.sh
```

@include parts/resolve.md

```text
bash .claude/skills/rebase/scripts/continue-rebase.sh
```

@include parts/resolve-after.md

## Step 5: verify the result

```text
bash .claude/skills/rebase/scripts/verify-rebase.sh
```

@include parts/verify.md
