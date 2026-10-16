---
name: write-pr-description
license: Apache-2.0
description: >-
  Write or revise the pull request description in PR_AGENTDESC.md, as a forked agent that reads the branch itself. Fills the frontmatter properties, a Conventional Commits title, and every section the repository's template declares, then runs the mechanical validator until it passes before returning. The pr skill calls this to draft, fix-pr calls it after remediation commits arrive, and merge-pr calls it for a final pass. Every call pairs with the pr review script afterwards.
context: fork
agent: general-purpose
background: false
---

# Write a pull request description

Write `PR_AGENTDESC.md` at the repository root, then return. You run forked, which keeps the branch diff in your context rather than the caller's. Read the material below and write from it.

## Arguments

`$ARGUMENTS` contains the repository root, and after it whatever the caller wants addressed. A first draft arrives with nothing extra. Any revision has the findings the pr review script returned, or a note that remediation added commits the description hasn't caught up with.

Treat any findings in the arguments as the work. Resolve each one and say so.

## Context

!`bash ${CLAUDE_SKILL_DIR}/scripts/context.sh`

@include parts/write.md

## Run the validator before returning

```text
bash .claude/skills/pr/scripts/validate-description.sh
```

Run it, fix every finding, and run it again until its output is empty. Each finding gives a line and the correction.

Returning a draft that fails the validator spends a review round on something a script settles by itself, and the caller only has to send you back, so treat this as part of writing rather than a step after it.

@include parts/return.md
