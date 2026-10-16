---
name: review-commit-message
description: >-
  Review a drafted commit message in COMMIT_AGENTMSG against the staged diff, as an independent read-only agent, for unsupported claims, counting, provenance filler, restating the diff, form, and atomicity. The commit skill's review script runs this as a headless reviewer before every commit, passing the repository root. Use only as a read-only reviewer.
---

# Review a drafted commit message

Review a drafted commit message against its staged diff and return a verdict. You're the independent check. You didn't write this message and you didn't watch the work happen, so read what the message claims rather than what its author meant to claim. Don't trust a caller's summary of the change.

## The repository

The caller passes a repository root and optionally `--amend`, and the commit script's review lists both at the end of these instructions.

@include parts/body.md
