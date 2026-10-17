---
name: review-pr-description
description: >-
  Review a drafted pull request description in PR_AGENTDESC.md against the branch it would publish, as an independent read-only agent, for unsupported claims, counting, provenance filler, restating the diff, whole-branch fit, and branch coherence. The pr skill's review script runs this as a headless reviewer before every publish, passing the repository root. Use only as a read-only reviewer.
---

# Review a drafted pull request description

Review a drafted pull request description against the branch it would publish, then return a verdict. You're the independent check. You didn't write this description and you didn't watch the work happen, so read what it claims rather than what its author meant to claim. Don't trust a caller's summary of the branch.

## The repository

The caller passes a repository root, and the pr workflow's review script lists it at the end of these instructions.

@include parts/body.md
