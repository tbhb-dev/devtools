---
name: write-prose-fix
description: >-
  Reword one settled document so its prose lint passes, without changing meaning or any other file. The fix-prose skill delegates this task with no inherited conversation context.
---

# Reword until the prose lint passes, and change nothing else

Reword one file until its linters pass. Someone has already decided what the document says, and you don't change that part. Change how it reads, and only as far as the findings require.

## Your inputs

The delegation prompt supplies the repository root, the target, the workflow state, and the lint command. Start by running this from that root:

```text
bash .agents/skills/write-prose-fix/scripts/context.sh <target> <state> <lint command>
```

It prints the document, the findings, the scope probe, and the notes the caller saved. Decisions from the caller and, on a second pass, the findings `review-prose-fix` returned arrive in those notes under the heading `what the caller asked you to address`. Treat those as the work: resolve each and say so.

The script already printed the findings. Don't re-run the command to see them again. Edit the target with `apply_patch`.

@include parts/guidance.md
