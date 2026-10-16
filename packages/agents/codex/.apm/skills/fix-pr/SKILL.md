---
name: fix-pr
description: >-
  Diagnose failing pull request checks, reproduce them locally, make focused fixes, commit them through commit, and refresh the published description when needed. Use whenever a pull request is red, the user asks Codex to fix CI, or watch-pr reports failures.
---

# Fix a failing pull request

Use `update_plan` for diagnosis, reproduction, fix, commit, description refresh, and push. The argument may specify a pull request and the chosen commit strategy.

## Diagnose and reproduce

Run once:

```bash
bash .agents/skills/fix-pr/scripts/diagnose.sh <number>
```

Stop if the checkout differs from the commit CI tested. For each failure, run the mapped local task before editing. If it passes, inspect the workflow for an environment mismatch, flake, incomplete task mapping, or stale base. Use `rebase` when the failing combined tree doesn't exist locally.

Fix only the cause. Don't weaken a rule, exclusion, or threshold without user approval. Re-run the reproducer until it passes.

## Choose and commit the fix

Honor a strategy supplied by `pr`. Otherwise, in Default mode ask in the final response whether to create a separate commit or amend and force-push. Don't commit in that turn. After confirmation, invoke `commit`. For an amendment, ensure its independent message review sees the complete amended commit.

## Refresh the description when material

Rebuild the local draft:

```bash
bash .agents/skills/fix-pr/scripts/populate-description.sh <number>
```

If the fix changes what readers need to know, delegate revision to `write-pr-description`, then delegate a fresh review to `review-pr-description`. Bound findings at three rounds. The reviewer records the digest after a successful review. Update the pull request only through:

```bash
bash .agents/skills/pr/scripts/create-pr.sh
```

Skip redrafting when the existing description remains accurate, and state that judgment.

## Push and continue

Push a separate commit normally. Push an amendment with `--force-with-lease`, never bare `--force`. Report the failed check, cause, fix, and new run. If asked to see CI through, use `watch-pr` and repeat. Stop after three failed rounds on the same check and report each attempt.
