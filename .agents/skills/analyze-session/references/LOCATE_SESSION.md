# Locating a session

Use this when the operator identified a session without giving a usable path. Searching the transcript stores in the calling session costs more context than the analysis that follows, so a cheap delegated agent does it and returns one line.

The smallest model tier is enough. Matching a file by its contents doesn't need judgment, and the answer is one path. `DISPATCH.md` in this directory shows how to launch the agent on this harness.

## The message

Fill in the search string, then send the agent this message:

```text
Find one agent transcript and return its absolute path. It may be a
Claude Code transcript or a Codex rollout; the tool searches both.

Run this, where <SKILL> is the analyze-session skill directory:

  <SKILL>/scripts/session.py locate --contains '<SEARCH STRING>'

It prints size, path, match count, and format per candidate, newest first.

Try these in order until one matches, and report which one worked:
  1. --contains '<SEARCH STRING>'
  2. --contains '<SECOND SEARCH STRING>'
  3. --project '<REPO NAME>' with no --contains, listing every
     transcript for that project so the caller picks by size and date

Return the absolute path alone when exactly one candidate matches.
Return every candidate with its size and match count when more than
one does. Say so plainly when nothing matches.

NEVER read a transcript. Each one is megabytes and would exceed your
context. The locate subcommand reads them line by line without
holding them in memory, which is why you run it instead of opening
files yourself.
```

## Choosing a search string

Pick a string appearing in that session and in no other. A filename the session created works well, and so does an identifier it invented or a rule name from its own output. A thread or session ID isn't automatically unique as content, because another session may quote it.

Avoid anything the repository already contains. A common word matches every transcript mentioning the project, and the agent returns a list rather than an answer.

## What to do with a null result

Zero hits proves nothing until the same string matches something else. A search matching no transcript may mean the session is absent, or may mean the string was wrong.

Give the agent a second string to try before concluding the session doesn't exist.

## A common mistake

Claude Code stores a worktree session's transcript under the main repository's project directory. The worktree's own directory under `~/.claude/projects/` may exist without containing any `.jsonl` at all, and it looks exactly like a missing transcript. Codex stores each session file in a dated directory, so the working directory never reveals the path.

Searching by content or by session metadata answers it correctly.
