---
name: fix-pr
description: >-
  Diagnose failing pull request checks, reproduce them locally, make focused fixes, commit them through commit, and refresh the published description when needed. Use whenever a pull request is red, the user asks Codex to fix CI, or watch-pr reports failures.
---

# Fix a failing pull request

Use `gh-post` for issue and PR content, comments, reviews, comment edits, and release notes. Supply body text through `--body-file` or stdin. If `gh-post` is missing, stop and report the missing command. Never fall back to a direct GitHub content write.

Where the harness provides `update_plan`, track diagnosis, reproduction, fix, commit, description refresh, and push with it. Otherwise work them in order as written. The argument may specify a pull request and the chosen commit strategy.

## Diagnose and reproduce

Run once:

```bash
bash .agents/skills/fix-pr/scripts/diagnose.sh <number>
```

Stop if the checkout differs from the commit CI tested. For each failure, run the mapped local task before editing. If it passes, inspect the workflow for an environment mismatch, flake, incomplete task mapping, or stale base. Use `rebase` when the failing combined tree doesn't exist locally.

Fix only the cause. Don't weaken a rule, exclusion, or threshold without user approval. Re-run the reproducer until it passes.

## Choose and commit the fix

Honor a strategy supplied by `pr`. Otherwise, in Default mode ask in the final response whether to create a separate commit or amend and force-push. Don't commit in that turn. After confirmation, invoke `commit`. For an amendment, pass `--amend` to the commit skill's review and commit scripts. The reviewer then reads the amended commit's whole content rather than the staged delta.

## Refresh the description when material

Rebuild the local draft:

```bash
bash .agents/skills/fix-pr/scripts/populate-description.sh <number>
```

If the fix changes what readers need to know, revise `PR_AGENTDESC.md` in this session or use the optional `write-pr-description` helper, then review it with `bash .agents/skills/pr/scripts/review.sh`, which stamps the draft on a pass. Bound findings at three rounds. Update the pull request only through:

```bash
bash .agents/skills/pr/scripts/create-pr.sh [--confirmed] [--rewrite]
```

It pushes the branch on the way. Pass `--confirmed` only where the operator chose a route including fixes, or answered now. After an amend, pass `--rewrite`, because the push replaces the commit on origin. It leases on that exact commit.

Skip redrafting when the existing description remains accurate, and state that judgment.

## Push and continue

Where you republished the description, `create-pr.sh` already pushed. Otherwise push a separate commit normally. Push an amendment with `--force-with-lease`, never bare `--force`. Report the failed check, cause, fix, and new run. If asked to see CI through, use `watch-pr` and repeat. Stop after three failed rounds on the same check and report each attempt.
