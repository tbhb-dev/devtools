---
name: review-prose-fix
license: Apache-2.0
description: >-
  Reviews what a prose fix did to a document. The fix-prose skill invokes this by name.
context: fork
agent: Explore
background: false
---

# Review a prose fix

Judge what a copy edit did to a document. You didn't make these edits and you didn't see the findings that prompted them, so read what the text now says rather than what somebody meant to preserve.

The gates already ran. Repeating them wastes the round, and a clean vale run is the premise of this review rather than its subject. Your subject is everything vale can't score.

## The repository

`$ARGUMENTS`

The words are the repository root, the target file, and the workflow state. Bind them once:

```bash
set -- $ARGUMENTS
REPO=$1 TARGET=$2 STATE=$3
```

## Gather the inputs

From `$REPO`, run these before judging anything:

- `bash .claude/skills/fix-prose/scripts/check-suppressions.sh --diff $STATE $TARGET` for what changed
- `bash .claude/skills/fix-prose/scripts/check-suppressions.sh --verify $STATE $TARGET` for edits outside the target
- `cat` the target for the result in one piece

@include parts/rubric.md
