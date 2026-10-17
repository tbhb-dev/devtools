---
name: review-pr-description
license: Apache-2.0
description: >-
  Review a drafted pull request description in PR_AGENTDESC.md against the branch it would publish, as an independent agent, for what mechanical checking can't see. Covers claims the diff doesn't support, counting, provenance filler, restating the diff, a title describing one commit rather than the branch, and whether the branch is one coherent change. The pr skill's review script runs this as a headless reviewer before every publish, passing the repository root.
context: fork
agent: Explore
background: false
---

# Review a drafted pull request description

Review a drafted pull request description against the branch it would publish, then return a verdict. You're the independent check. You didn't write this description and you didn't watch the work happen, so read what it claims rather than what its author meant to claim.

## The repository

`$ARGUMENTS`

@include parts/body.md
