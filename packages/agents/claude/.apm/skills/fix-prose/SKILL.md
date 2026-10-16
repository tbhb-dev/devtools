---
name: fix-prose
license: Apache-2.0
description: >-
  Clear the vale and cspell findings on a document whose content is already settled. A forked writer does the rounds and an independent reviewer reads the result. The finding text never enters this session's context. Takes the file plus the lint command that judges it. Use it when a draft is written but its prose gates are still red, or when a lint task has come back with findings more than once.
hooks:
  PreToolUse:
    - matcher: Write|Edit
      hooks:
        - type: command
          command: "${CLAUDE_PROJECT_DIR}/.claude/skills/fix-prose/scripts/guard-target.sh"
  PostToolUse:
    - matcher: Skill
      hooks:
        - type: command
          command: "${CLAUDE_PROJECT_DIR}/.claude/skills/fix-prose/scripts/arm-guard.sh"
---

# Reword a settled draft until its prose lint passes

Run two forked skills against each other until the document holds up. Findings, retries, and rejected wordings stay in their context rather than this one. That's the whole reason the skill exists. A twenty-line commit message can cost four rounds of vale output, and this session needs none of it in its context.

Use it once the document says what it should say. The writer changes wording rather than claims, so a draft still arguing with itself needs its author instead.

## Preflight

!`bash ${CLAUDE_SKILL_DIR}/scripts/preflight.sh $ARGUMENTS`

Read the verdict before anything else. `BLOCKED` there means no agent can help. The file is missing, no rule set covers the path, or the document is already clean. Say which and hand back.

The preflight prints counts and verdicts rather than findings, on purpose. This body loads into your context rather than a fork, so a finding printed here enters the context the skill exists to keep clear.

## Arguments

Give the file first, then the command that judges it.

```text
COMMIT_AGENTMSG mise run lint-commit-msg
PR_AGENTDESC.md mise run lint-pr-description
docs/design.md mise run lint-draft docs/design.md
```

Only the caller knows which gate applies, and it supplies the command. A commit draft takes the commit-msg scope, and a description takes its own task. Anything else takes `mise run lint-draft`, which runs vale and cspell and rumdl together over one file.

Prefer a task that runs every checker at once. `mise run lint-prose` runs vale alone, so a document clearing it can still fail spelling and structure, and a verdict from that task is narrower than it sounds.

Where the request doesn't specify a file, ask which one.

## Step 1: record the baseline

```bash
bash .claude/skills/fix-prose/scripts/check-suppressions.sh --baseline <target>
```

It prints a `state:` path, a directory inside the worktree that git ignores. Keep that path and pass it to the writer and the reviewer. It holds the target as it stood before the writer started, which the reviewer reads against the result, and the record of every dictionary and rule file the later check compares.

## Step 2: fix

Invoke `write-prose-fix`. It runs forked, and its own context block hands it the findings and the document, so it starts working rather than looking.

```text
Skill(write-prose-fix, args: "<target> <state> <lint command>")
```

Pass nothing else in the arguments. The step that builds the writer's context substitutes them into a shell command line as raw text. There a semicolon runs, a parenthesis fails to parse, and an asterisk expands against the working directory. Findings text contains all three.

Where you have findings or a decision to hand over, write them to the notes file in the state directory and leave the arguments alone:

```bash
printf '%s\n' "<the reviewer's findings or the decision, verbatim>" > "<state>/notes"
```

The writer's context prints that file and works from it. It lives in the worktree rather than the git directory because a session isolated in a worktree can't write into the git directory. The context script falls back to the older location, `fix-prose.notes` under the git directory, when the run has no notes.

Never edit the target yourself. A guard hook refuses it, because editing it here means reading the findings here.

Where a finding needs a decision or a rule flags correct prose, the guard's refusal prints a command that releases one edit, and that edit is yours.

It returns `PROSE: CLEAN`, `PROSE: PARTIAL` with a short unresolved list, or `PROSE: BLOCKED`.

## Step 3: check for silenced checkers

```bash
bash .claude/skills/fix-prose/scripts/check-suppressions.sh --verify <state> <target>
```

Silence means the writer edited nothing but the target. Any output identifies a file it shouldn't have changed.

This runs here rather than inside the fork because an agent certifying its own work certifies nothing. The writer runs unguarded in there, so it can edit a style file or add an inline `cspell:ignore`, and both produce a clean lint run over a document nobody fixed. A finding means reverting that edit and taking the original lint finding back as unresolved.

## Step 4: review, and loop

Invoke `review-prose-fix`. It runs forked as an independent agent that didn't see the findings or the edits, and it reads the before and after against each other.

```text
Skill(review-prose-fix, args: "<repository root> <target> <state>")
```

The review exists because a clean lint run is a weak result on its own, and no gate scores a claim that moved, a name traded for a vaguer phrase, or a metaphor that arrived while somebody was clearing a rule. A document can pass every checker and come back saying something its author never wrote.

Act on its verdict:

- `VERDICT: PASS` ends the loop. Go on to the final lint.
- `VERDICT: CHANGES REQUIRED` returns to step 2 with the findings verbatim in the notes file.

Bound it at three review rounds. Stop after that and report which findings keep returning, because a fourth pass at the same objection rarely helps. Run the verification in step 3 again after every writer round.

## Sanctioning a dictionary word

The writer never edits a dictionary. A token it reports as unresolved waits for a decision. When the operator decides the word belongs, add the line to `.cspell-words.txt` or to the vale vocabulary's `accept.txt`, then record exactly that line against the run:

```text
check-suppressions.sh --allow <state> .cspell-words.txt <word>
```

The verification then passes on those added lines and still fails on anything else, including a removed word, an extra word, or an edit to a rule file. The operator can sanction lines in only those two dictionary files. Do it only on the operator's word, never on the writer's suggestion.

## Final lint and cleanup

Run the complete lint command once more, with its output discarded, and read only its exit status. Whether the gates pass is the question here, and a finding printed in this context defeats the point of the skill.

Always run `check-suppressions.sh --cleanup <state>` before returning, once no agent needs the state, even when the outcome is unresolved or blocked.

## What to report

Report `PROSE: CLEAN` only when the writer reported clean, the suppression check is silent, the reviewer returned `VERDICT: PASS`, and the final lint command passes. Otherwise return the unresolved or blocked report and nothing else.

Each unresolved entry needs a decision that changes what the document claims, which is yours or the operator's rather than another lint round. A rule matching correct text arrives on that list by design. The writer never turns a rule off or adds an exception, and never rewrites a sentence into nonsense to clear one.

## Safety rules

- Only the writer edits the target, and the target is the only file it may edit.
- Never change lint configuration, dictionaries, hooks, or suppression comments on the writer's behalf.
- Don't infer clean from silent output unless the preflight proved the target has prose rules.
- Don't print full lint findings in this session unless the workflow blocks and the operator needs a decision.
