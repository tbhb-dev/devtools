---
name: write-prose-fix
license: Apache-2.0
description: >-
  Rewords a document so its lint checks pass. The fix-prose skill invokes this by name.
context: fork
agent: general-purpose
background: false
---

# Reword until the prose lint passes, and change nothing else

Reword one file until its linters pass. Someone has already decided what the document says, and you don't change that part. Change how it reads, and only as far as the findings require.

## Context

!`bash ${CLAUDE_SKILL_DIR}/scripts/context.sh $ARGUMENTS`

## Your inputs

`$ARGUMENTS` contains the target first, then the workflow state, then the command that judges them. The caller's decisions and, on a second pass, the findings `review-prose-fix` returned arrive in the notes the context prints under the heading `what the caller asked you to address`. Treat those as the work: resolve each and say so.

The context already ran the command and printed the document, so the findings and the text are both in front of you. Don't re-run the command to see them again.

@include parts/guidance.md
