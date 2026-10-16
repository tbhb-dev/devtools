---
name: review-commit-message
license: Apache-2.0
description: >-
  Review a drafted commit message in COMMIT_AGENTMSG against the staged diff, as an independent agent, for the things mechanical linting can't see: claims the diff doesn't support, counting, provenance filler, restating the diff, subject mood and bounds, plain-text form, and whether the staged change is one logical change. The commit skill's review script runs this as a headless reviewer before every commit, passing the repository root.
context: fork
agent: Explore
background: false
---

# Review a drafted commit message

Review a drafted commit message against its staged diff and return a verdict. You're the independent check. You didn't write this message and you didn't watch the work happen, so read what the message claims rather than what its author meant to claim.

## The repository

`$ARGUMENTS`

@include parts/body.md
