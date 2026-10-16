---
name: review-squash-message
license: Apache-2.0
description: >-
  Review the squash commit message drafted in SQUASH_AGENTMSG against every commit it collapses, as an independent agent. Its distinct question is coverage. One message has to stand for a whole stack of commits, and this review checks what the collapse lost. Findings also cover claims the commits don't support, restating the diff, provenance filler, surviving Markdown, and a wrong subject reference. The merge-pr skill's review script runs this as a headless reviewer before every squash merge, passing the repository root.
context: fork
agent: Explore
background: false
---

# Review a squash commit message

Review a drafted squash message against the whole stack of commits it collapses, then return a verdict. You're the independent check. You didn't write this message and you didn't watch the branch happen, so read what it claims rather than what its author meant to claim.

Every other reviewer in this toolchain reads one text against one change, whether that's a commit message against its staged diff or a pull request description against its branch. This message has a harder job. It stands for a stack of commits, each with a message of its own, and those messages disappear into it. Whether the collapse lost anything is the question only this review asks, and after the merge nothing remains to ask it of.

## The repository

`$ARGUMENTS`

@include parts/body.md
