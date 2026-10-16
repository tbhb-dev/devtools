---
name: fix-prose
description: >-
  Clear vale and cspell findings on a settled document through an isolated writer and an independent reviewer. Use when a draft's prose gates are red or repeated lint findings would pollute the parent context. Takes a target and its complete lint command.
---

# Reword a settled draft until its prose lint passes

Keep findings and retries out of the parent context. Findings, retries, and rejected wordings stay in the delegated agents, and the parent sees only verdicts. Use the skill once the document says what it should say. The writer changes wording rather than claims.

## Preflight

Parse the request into a target and a complete lint command, then run this from the repository holding the target:

```text
bash .agents/skills/fix-prose/scripts/preflight.sh <target> <lint command>
```

Stop on `BLOCKED` and say which condition applies. The preflight prints counts and verdicts rather than findings, on purpose.

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

## The checklist

Where the harness provides `update_plan`, track these with it, keeping exactly one item `in_progress`. Otherwise work the list in order as written.

1. Record the baseline
2. Fix with the writer
3. Check for silenced checkers
4. Review independently
5. Lint and clean up

## Step 1: record the baseline

```text
bash .agents/skills/fix-prose/scripts/check-suppressions.sh --baseline <target>
```

It prints a `state:` path, a directory inside the worktree that git ignores. Save the absolute path. It identifies this workflow and records its target, and until cleanup the script refuses a second workflow on the same target. Pass the state to every delegated writer and reviewer in this run.

## Step 2: fix

Delegate `write-prose-fix` with `fork_turns: "none"`. Give it only the repository root, target, workflow state, lint command, and an instruction to follow that skill. Wait for its terminal report.

Where you have findings or a decision to hand over, write them to the notes file in the state directory before delegating:

```text
printf '%s\n' "<the reviewer's findings or the decision, verbatim>" > "<state>/notes"
```

The writer's context prints that file and works from it. The context script falls back to the older location, `fix-prose.notes` under the git directory, when the run has no notes.

Never edit the target in the parent. It returns `PROSE: CLEAN`, `PROSE: PARTIAL` with a short unresolved list, or `PROSE: BLOCKED`.

## Step 3: check for silenced checkers

```text
bash .agents/skills/fix-prose/scripts/check-suppressions.sh --verify <state> <target>
```

Silence means the writer edited nothing but the target. Any output identifies a file it shouldn't have changed. This runs in the parent because an agent certifying its own work certifies nothing. A finding means reverting that edit and taking the original lint finding back as unresolved.

## Step 4: review, and loop

Delegate `review-prose-fix` with `fork_turns: "none"`, and give it the repository root, target, and workflow state. It runs as an independent agent that didn't see the findings or the edits, and it reads the before and after against each other.

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
