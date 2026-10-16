---
name: review-prose-fix
description: >-
  Independently review a prose-lint fix for changed meaning, lost precision, unrelated edits, suppression comments, and language defects the linter can't score. The fix-prose skill delegates this after the writer finishes. Use only as a read-only reviewer.
---

# Review a prose fix

Judge what a copy edit did to a document. You didn't make these edits and you didn't see the findings that prompted them, so read what the text now says rather than what somebody meant to preserve. Work read-only, and don't edit any file.

The gates already ran. Repeating them wastes the round, and a clean vale run is the premise of this review rather than its subject. Your subject is everything vale can't score.

## The repository

The caller supplies the repository root, the target file, and the workflow state in the delegation prompt.

## Gather the inputs

From the repository root, run these before judging anything:

- `bash .agents/skills/fix-prose/scripts/check-suppressions.sh --diff <state> <target>` for what changed
- `bash .agents/skills/fix-prose/scripts/check-suppressions.sh --verify <state> <target>` for edits outside the target
- read the target in full for the result in one piece

@include parts/rubric.md
