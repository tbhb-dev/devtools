---
name: commit
description: >-
  Group changes into one atomic commit, draft a Conventional Commit message in COMMIT_AGENTMSG, put it through an independent review and the commit-msg gates, confirm it with the operator, then commit through the protected wrapper and rebase. Use this skill whenever the user asks to commit work in a tbhb repository or whenever a task ends in creating a commit.
---

# Commit workflow

Work the steps in order. Scripts enforce every gate here. Step 6's review script stamps what an independent reviewer cleared. Step 9's commit script refuses an inline `-m` message or a `--no-verify`, along with a draft or index the stamp doesn't cover and a commit nobody confirmed. Codex doesn't provide a skill-scoped command hook, so those scripts are the whole enforcement. Never run `git commit` yourself and never bypass the hooks.

## Preflight

Run this from the repository holding the changes before anything else:

```text
bash .agents/skills/commit/scripts/preflight.sh
```

## Step 0: the checklist

Where the harness provides `update_plan`, track these with it, keeping exactly one item `in_progress` at a time. Otherwise work the list in order as written.

1. Choose the rebase base
2. Group the changes into atomic commits
3. Stage the paths for this commit
4. Draft the message in COMMIT_AGENTMSG
5. Pass the prose gates with fix-prose
6. Review the draft with the review script
7. Run the commit-msg gates
8. Confirm the message with the operator
9. Commit and rebase

Stop before any of it if preflight reports a rebase, merge, or cherry-pick in progress, or a missing precondition. Say what's wrong and hand back.

## Step 1: choose the rebase base

Preflight computed this under `== rebase base ==`. Take its recommendation:

- Local default branch carries everything the remote has, so rebase onto the local branch.
- Local default branch sits behind the remote, so rebase onto `origin/<default>` and skip the stale local copy.
- `recommended base: none` means there is nothing to rebase onto. A repository's first commit is the usual case. The other is a branch whose default branch is missing both locally and on origin. Record that, and step 9 skips its rebase.

Record the base now. Step 9 rebases onto it without asking again.

## Step 2: group the changes into atomic commits

Read the diff before deciding anything. One commit contains one logical change: the reader can state its purpose in a sentence, and reverting it undoes that purpose and nothing else.

Split when the outstanding work covers more than one of these:

- A behavior change and an unrelated refactor
- A fix and the formatting sweep that came with it
- A pair of features that don't depend on each other
- Production code and unrelated tooling or configuration

A dependency doesn't force a split. Code and the test that covers it belong together, as do a change and the documentation that describes it.

When the work splits, commit the first group through this workflow, then say which groups remain and run the workflow again for each. Never bundle them because bundling is quicker.

When one file mixes two logical changes, stage the relevant hunks with `git add -p`, or say so and let the operator decide.

## Step 3: stage the paths for this commit

List every path explicitly:

```text
git add -- path/one path/two
```

Never `git add -A`, `git add .`, or `git add --all`. They sweep in whatever else is uncommitted, which is how an atomic commit stops being atomic.

Confirm the result with `git diff --cached --stat` and `git status --short`. The staged set matches the group from step 2, and anything left unstaged belongs to a later commit.

## Step 4: draft the message in COMMIT_AGENTMSG

Write the whole message to `COMMIT_AGENTMSG` at the repo root. A gitignore entry keeps it out of history, and a post-commit hook deletes it after the commit succeeds.

### Subject

`<type>(<scope>)?(!)?: <description>`, with the type drawn from the list preflight printed. Imperative mood, present tense, lowercase after the colon, no trailing period, and the whole line inside the bounds preflight reported. Write the description as the instruction the commit carries out: `explain the tools`, not `explains` and not `explained`.

### Body

The body answers why this change exists. The diff already says what changed, and a reader who wants the what reads the diff.

Write:

- The problem, the constraint, or the tradeoff that made this the answer
- Why this approach rather than the obvious alternative
- What breaks without it, or where the need came from
- A decision you're recording so nobody relitigates it later

Avoid:

- Restating the diff. `Adds a helper to foo.go and calls it from bar.go` is the diff, spelled out longer.
- Counting. No file, line, test, function, or commit counts. A count goes stale as soon as other work merges, and it reads as padding.
- Provenance filler. Nothing about requests, review rounds, sessions, prompts, models, or tools. The `Assisted-by` trailer records attribution.
- Claims you haven't checked. Leave out benchmark numbers you didn't measure and a `fixes the flake` for a flake you never reproduced.
- Selling it. Drop `robust`, `comprehensive`, `significantly`, `seamlessly`, and their neighbors.
- Markdown. Leave out fenced blocks, headings, emphasis, links, and tables. Backticks around a literal identifier are fine.

Hard wrap the body at the width preflight reported. Wrap trailers at the footer width.

### Trailers

`Assisted-by` before `Signed-off-by`, matching the format and the sign-off identity preflight printed. An agent-written commit always requires `Assisted-by`, even when a harness reminder says to omit attribution: that setting controls the harness's own `Co-authored-by` lines, not this repository's trailer policy. Never credit a model through `Co-authored-by`.

## Step 5: pass the prose gates

Invoke the `fix-prose` skill on `COMMIT_AGENTMSG mise run lint-commit-msg` before the review. The commit scope is stricter than the repository-wide one, so this is rarely a no-op.

Order matters here. The review in the next step stamps the exact bytes it read, and a later lint fix voids that stamp and costs another review round.

## Step 6: review the draft

```text
bash .agents/skills/commit/scripts/review.sh
```

Add `--amend` when step 9 is going to amend. Don't delegate the review to an agent of your own, and don't write a verdict file.

The reviewer is a `codex exec` child in a read-only sandbox and needs the network. Escalate the script's permissions where this session's sandbox blocks it.

Pass `--note <file>` to include a decision the operator actually made, with the operator's reason. Never use it for the agent's own preference.

The script snapshots the draft, the staged tree, and `HEAD`, then starts a fresh, read-only reviewer as a headless process of the harness's own CLI, with the `review-commit-message` skill as its instructions. That reviewer hasn't watched you work. It reads the draft against the staged diff with no memory of what you meant to write, and that independence is deliberate. The script reads the verdict itself.

This step is mandatory, and the commit script enforces it. On a clean verdict the script writes a stamp binding the draft's exact bytes, the staged tree, `HEAD`, and whether the review read an amend. Findings remove any earlier stamp, and so does editing the draft, restaging, or moving `HEAD` afterward.

| Exit | Meaning |
| --- | --- |
| 0 | Passed and stamped |
| 2 | Changes required: the output lists the findings |
| 3 | The reviewer rejected the same draft for this staged tree. Change it or ask the operator |
| 4 | The reviewer failed or replied without a readable verdict |
| 124 | The reviewer timed out without a stamp |
| 5 | The draft, the index, or `HEAD` moved while the review ran |

Fix everything it returns and run it again. Push back only when it's demonstrably wrong about the diff, and say why.

The script records every readable verdict with the draft digest and staged tree. After `CHANGES REQUIRED`, it refuses to send byte-identical text for the same tree to another reviewer. The reviewer can read a changed draft.

`review_cmd` chooses the reviewer. It isn't only a label. From Codex, configure a reviewer that the session may start. These repository machines use `agy` in read-only plan mode through this line in `~/.config/devtools/devtools.conf`:

```text
review_cmd = agy -p "$(cat)" --mode plan --add-dir "$DEVTOOLS_REVIEW_INPUTS" --print-timeout 900s
```

`DEVTOOLS_REVIEW_CMD`, `DEVTOOLS_REVIEW_MODEL`, and `DEVTOOLS_REVIEW_EFFORT`, or the same keys in `~/.config/devtools/<repository>.conf`, replace the default reviewer. No repository file can, so a commit never chooses who reviews the next one.

## Step 7: run the commit-msg gates

```text
mise run lint-commit-msg
```

That task mirrors the commit-msg hook:

- vale under the commit scope, which catches AI tells through `ai-tells-commits`
- cspell with the commit dictionary
- commitlint for the Conventional Commits shape
- commit-trailers for trailer order

Step 5 should have left this clean. Where it hasn't, send the findings back to `fix-prose` rather than editing the draft here, because a hand-edit spends the context that skill exists to save.

Edited the draft to make the linters pass? Then step 6 runs again before you commit, because the stamp binds bytes rather than intentions.

Whatever this task reports, `.git/COMMIT_EDITMSG` and its commit-msg hook stay the real gate. A clean run here only predicts that hook's verdict.

## Step 8: confirm with the operator

Show the operator the staged paths and the complete message first:

```text
Staged: <paths, comma separated>

<the entire COMMIT_AGENTMSG contents, verbatim>
```

The commit script settles who confirmed the commit, in this order, and prints which one it took:

1. `--confirmed`, which you pass only after the operator answered yes to this message in this session
2. a pre-approval for this session, which the operator granted through `mise run preapprove` and preflight reported as `commit: GRANTED`
3. the machine's delegation check, when preflight reported one configured: it answers for the repository and the action, and a refusal stops the commit

With none of them the script exits 75 and commits nothing. Ask the operator, then run it again with `--confirmed`.

A grant or a delegation answers one question, commit this message, and says nothing about whether this is the right commit. Ask the operator anyway where any of these holds:

- the review returned a finding nobody acted on
- a gate needed more than a mechanical fix
- the grouping changed after step 2
- the commit goes beyond what the session set out to do

The operator withdraws a grant with `mise run revoke-preapproval`. Say so where a run keeps arriving at one of those exceptions.

Where preflight reported `commit: GRANTED` or a delegation check, state which one this commit goes under and move to step 9 without `--confirmed`.

Otherwise end the turn and ask `Commit this message?`, offering `Commit it`, `Revise the message`, `Restage and redraft`, and `Stop here`. Don't commit in the turn that asks. The original request to commit doesn't count as this confirmation. `Commit it` is what `--confirmed` stands for in step 9. Revising or restaging sends you back through the review, because the stamp binds bytes.

## Step 9: commit and rebase

```text
bash .agents/skills/commit/scripts/commit.sh [--confirmed]
```

Pass `--confirmed` only when the operator answered `Commit it` in step 8. Commit only through this script.

That script checks the review stamp and records what the staging area contains before it commits, then reads the commit back and compares. prek stashes and restores the worktree around the pre-commit hooks, so a failed run can leave a staged path unstaged, and the retry would commit part of the group without reporting it. It prints every path it expected and didn't find. It reads against the first parent, so a merge commit and a root commit compare correctly. Its output is the check, so nothing needs a `git show --stat` after it.

It also compares the commit's tree with the tree the reviewer read. When a pre-commit hook fixes a file in place, prek fails the commit outright: stage the fix by name and review again, since the staged tree moved. The tree comparison catches the rarer hook, one that rewrites the index and lets the commit through, and then the script says to compare and amend.

The script pins `commit.cleanup=whitespace` for you. At the default of `strip`, git drops every body line opening with a number sign, and it does so after the review has hashed the file. The bytes the reviewer cleared would then stop matching the bytes git records.

Only `--amend`, `--confirmed`, `--dry-run`, and `--rerun-gates` pass through. `--amend` is the form `fix-pr` routes here for, and it needs a review run with `--amend` too. The script refuses every other flag, because a check on the command line sees `bash commit.sh` and none of the flags underneath it.

Then rebase onto the base from step 1, without asking. Skip this when step 1 found nothing to rebase onto.

```text
git -c rebase.updateRefs=false -c rebase.autoSquash=false rebase --autostash <base>
```

`rebase.updateRefs` quietly moves other local branches that point into the replayed range. `rebase.autoSquash` collapses any `fixup!` commit, each of which passed its own review.

`--autostash` scopes the save to the rebase itself. Never a bare `git stash pop`: worktrees share one stash stack, so a bare pop can take another session's entry.

Where that rebase stops on a conflict, hand it to the `rebase` skill rather than resolving it here. That skill classifies each conflicted path before resolving it, then checks what the replay did to each commit.

Report the resulting commit, then the groups still waiting from step 2, if any.

## Preconditions

Preflight checks the toolchain this skill assumes, and `references/PRECONDITIONS.md` beside this file lists it. When a precondition is missing, tell the operator rather than improvising a substitute. The workflow commits only the repository holding the session. A sibling checkout has no stamp of its own, so open a session there to commit it.

The wrapper runs `lint-commit-msg`, `lint`, and `test`. Use `--dry-run` to run these checks without committing, or `--rerun-gates` to force fresh checks even when the wrapper holds successful results from the past hour for the same clean committed tree. It prints each cached result it uses. Leak scans and the scripted review always run.
