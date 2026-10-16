---
name: analyze-session
description: >-
  Query a Claude Code or Codex transcript without reading it into context, then write the retrospective to disk so it remains available after the session ends. The tool reads the file line by line and reports every failure and repeated tool call by line number. Use this skill when the user asks for a retrospective on a past session, or hands over a transcript path with a question about it.
---

# Analyze a session

Work the steps in order. This skill ends once the retrospective exists on disk and the operator has its path. Acting on what it recommends is separate work. Use the `commit` skill for any change that comes out of it.

Paths below, such as `scripts/session.py`, are relative to this skill's directory, `.agents/skills/analyze-session/`.

## Which session

Use the absolute transcript path from the user's request. It comes from either harness. A bare session ID also works when it matches exactly one file under `~/.codex/sessions/` or `~/.claude/projects/`, which is enough.

If the user gives neither, preflight explains how to find a session. Spawn a fresh delegated agent with `fork_turns: "none"`. Run preflight again with the path it returns. Never load transcripts into model context merely to locate one, because the locator can stream them.

## Preflight

Run this before anything else and read its report:

```text
bash .agents/skills/analyze-session/scripts/preflight.sh <path-or-id>
```

## Step 0: open the task list

Use `update_plan` where the harness provides it, moving each item to `in_progress` and `completed` as you go. Otherwise work the list in order as written. They're the checklist the rest of this document expands.

@include parts/checklist.md

The preflight already resolved the transcript and summarized the session. It created the retro directory at `<main worktree>/.codex/retros/YYYYMMDD_<session ID>/` and copied the evidence into it. Read that report before forming any hypothesis, because it writes everything it prints to `artifacts/` as well, where a later reader can check it.

`references/SESSION_CLI.md` documents every subcommand, for when a report needs more detail than the preflight printed.

@include parts/steps.md
