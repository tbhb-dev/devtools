---
name: analyze-session
license: Apache-2.0
description: >-
  Query a Claude Code or Codex transcript without reading it into context, then write the retrospective to disk so it remains available after the session ends. The tool reads the file line by line and reports every failure and repeated tool call by line number. Use this skill when the user asks for a retrospective on a past session, or hands over a transcript path with a question about it.
---

# Analyze a session

Work the steps in order. This skill ends once the retrospective exists on disk and the operator has its path. Acting on what it recommends is separate work. Use the `commit` skill for any change that comes out of it.

Paths below, such as `scripts/session.py`, are relative to this skill's directory, `.claude/skills/analyze-session/`.

## Which session

`$ARGUMENTS` holds the absolute path to the transcript. It comes from either harness. A bare session ID also works, provided it matches exactly one file under `~/.claude/projects/` or `~/.codex/sessions/`, which is enough.

If you give neither, the preflight says so and prints how to find it. Send a Haiku subagent, then run the preflight again with the path it returns. Never search the transcript directories yourself. Reading transcripts to find one costs more context than the entire analysis that follows it.

## Preflight

!`bash ${CLAUDE_SKILL_DIR}/scripts/preflight.sh ${ARGUMENTS}`

## Step 0: the checklist

Track these steps with the session's task-list tools where it has them. Newer harnesses leave those tools out by default, and a session without them works the list in order as written. They're the checklist the rest of this document expands.

@include parts/checklist.md

The preflight already resolved the transcript and summarized the session. It created the retro directory at `<main worktree>/.claude/retros/YYYYMMDD_<session ID>/` and copied the evidence into it. Read that report before forming any hypothesis, because it writes everything it prints to `artifacts/` as well, where a later reader can check it.

`references/SESSION_CLI.md` documents every subcommand, for when a report needs more detail than the preflight printed.

@include parts/steps.md
