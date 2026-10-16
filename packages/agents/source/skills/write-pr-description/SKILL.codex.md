---
name: write-pr-description
description: >-
  Write or revise the pull request description in PR_AGENTDESC.md, as a delegated agent that reads the branch itself. Fills the frontmatter properties, a Conventional Commits title, and every section the repository's template declares, then runs the mechanical validator until it passes before returning. Use as the delegated writer from pr, fix-pr, or merge-pr, with review findings or branch-drift notes in the task. Every call pairs with the pr review script afterwards.
---

# Write a pull request description

Write `PR_AGENTDESC.md` at the repository root, then return. You run as a delegated agent, which keeps the branch diff in your context rather than the caller's. Read the material below and write from it.

## Arguments

The task specifies the repository root, and after it whatever the caller wants addressed. A first draft arrives with nothing extra. Any revision has the findings the pr review script returned, or a note that remediation added commits the description hasn't caught up with.

Treat any findings in the task as the work. Resolve each one and say so.

## Context

Gather it once, before writing anything:

```bash
bash .agents/skills/write-pr-description/scripts/context.sh
```

@include parts/write.md

## Run the validator before returning

```bash
bash .agents/skills/pr/scripts/validate-description.sh
```

Run it, fix every finding, and run it again until its output is empty. Each finding gives a line and the correction.

Returning a draft that fails the validator spends a review round on something a script settles by itself, and the caller only has to send you back, so treat this as part of writing rather than a step after it.

@include parts/return.md
