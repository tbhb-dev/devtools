---
name: merge-pr
license: Apache-2.0
description: >-
  Squash merge a pull request. The commit message comes from this workflow rather than from GitHub, whose own version concatenates every commit on the branch into text no linter ever reads. A briefing script prints the published description, every commit the squash collapses, and the diffstat, then leaves a SQUASH_AGENTMSG skeleton whose body the caller writes. The commit-msg gates, an independent review, and an operator confirmation all run before the merge. Use this whenever the user asks to merge or land a pull request, including one nobody here authored such as a dependency bump.
hooks:
  PreToolUse:
    - matcher: Bash
      hooks:
        - type: command
          command: "${CLAUDE_PROJECT_DIR}/.claude/skills/merge-pr/scripts/guard-merge.sh"
---

# Merge a pull request

Work the steps in order. Scripts enforce every gate here. Step 5's review script stamps what an independent reviewer cleared, bound to the pull request's head and base. Step 7's merge script refuses a message the stamp doesn't cover, a pull request that moved or isn't ready, and a merge nobody confirmed, then merges pinned to the reviewed head. Both hold in any session, including a subagent's.

A guard hook runs alongside them as a rail, refusing a direct `gh pr merge` and every other `gh pr` verb that changes a pull request. Reading remains permitted.

Left alone, GitHub writes the squash message by concatenating every commit on the branch. That text has never passed a commit-msg hook, and it arrives on the default branch where the rest of the toolchain assumes those hooks ran. The toolchain never lints it after the merge, either. This workflow writes the message itself and puts it through the same checks a commit must pass.

## Preflight

!`bash ${CLAUDE_SKILL_DIR}/scripts/preflight.sh ${ARGUMENTS}`

## Step 0: the checklist

Track these steps with the session's task-list tools where the session provides them. Newer harnesses leave those tools out by default, and a session without them works the list in order as written.

1. Confirm the pull request is mergeable
2. Settle the published description
3. Write the squash message in SQUASH_AGENTMSG
4. Run the commit-msg gates
5. Review the message with the review script
6. Confirm the message with the operator
7. Merge and clean up

Stop before any of it where preflight reports a missing precondition, a draft pull request, or a failing check. Say what's wrong and hand back.

## Step 1: confirm the pull request is mergeable

Preflight printed the state, the checks, and the review decision. Merging needs an open pull request that nobody marked as a draft, with every check green.

For each check, preflight judges its latest run. A workflow that started twice on one head and cancelled the first run leaves those cancelled jobs behind, and preflight leaves them out and says how many it left out. Without a later run, a cancelled run still counts as failing.

A failing check goes to `fix-pr`, and a running one goes to `watch-pr`. Neither belongs here, and the merge script refuses both anyway.

A `mergeStateStatus` of `BEHIND` or `DIRTY` is the third case. The branch needs the base underneath it before anything merges, so run the `rebase` skill and push the result. Don't reach for GitHub's own update button: it merges the base into the branch, which leaves a merge commit this workflow never wrote a message for.

For `REVIEW_REQUIRED` or `CHANGES_REQUESTED`, preflight exits 75 with `WAITING_REVIEW`. Resolve the required approvals, then rerun. Review checks before invoking the reviewer. Merge checks again before the merge state.

## Step 2: settle the description first

The description is on GitHub at this point, and often nothing sits on disk. Fetch it back before revising it:

```text
bash .claude/skills/merge-pr/scripts/populate-description.sh <number>
```

That rebuilds `PR_AGENTDESC.md` from what the pull request currently says, reassembling the frontmatter from the properties set on the pull request. It refuses to overwrite an existing draft without `--force`, because a `pr` workflow further up the stack may have one in flight.

Merging is the last moment the description can change, and you write the squash message from it, so a wrong description propagates that error into history.

Read what preflight printed under `== description as published ==` against the commits it collapses. Most merges skip the rest of this step. A description that still fits doesn't need a pass, and a pull request nobody here authored has none of this machinery behind it, so take its description as it stands.

Where it no longer describes the branch, revise `PR_AGENTDESC.md` in this session or use the optional `write-pr-description` helper, then run the `pr` review script, then republish with `bash .claude/skills/pr/scripts/create-pr.sh`.

## Step 3: write the squash message

```text
bash .claude/skills/merge-pr/scripts/squash-message.sh <number>
```

That prints a briefing and writes a skeleton, but it doesn't convert the description into a message, because no rewriting rule produces one. The commit that merges here is the one a whole branch leaves behind, so writing it means reading every commit on the branch and working out what a reader years from now still needs.

Your briefing carries the description as published, every commit message the squash collapses in full, the diffstat, and the trailers on those commits. Read it before writing a word. Commit bodies matter most. Each one already argued for itself once, and this is the last place that argument still appears. Run the command on its own and read the whole thing. Piping it through `head` or `tail` keeps the trailing skeleton and drops those bodies, which nothing else in this workflow prints, and the script caps nothing on purpose, so a cap you add is one nobody downstream can see you added.

Its skeleton goes to `SQUASH_AGENTMSG` at the repository root, a file a gitignore entry keeps out of history. Subject and footer come filled in, and the body is empty until you write it.

### Subject

The script sets `<pull request title> (#<number>)`. Keep the reference, because `squash-merge.sh` reads it back to confirm the draft belongs to this pull request. Fix the rest where the title described one commit rather than the branch. A squash leaves one commit behind. Its subject has to describe what the whole stack did.

### Body

The description was Markdown, written for a reviewer with the diff open beside it. This is plain text, and whoever reads it three years from now has neither.

Take from the description:

- Summary and Why, which state the reason the branch exists
- Risk, but only where it describes a rollback the reader wouldn't guess

Leave behind:

- Verification, entirely. It reported what a reviewer needed at the time, and the moment the suite changes it describes a run nobody can repeat.
- Risk that only says the change could be wrong.

Then read the result back against the commits. Trust the commits over it. Authors publish descriptions early while branches keep growing, and the commits record what landed rather than what someone expected to land. Where a commit body gives a reason the description never stated, that reason belongs in the message. Otherwise it goes at the merge, with the commit that held it.

Avoid:

- Restating the diff. A reader who wants the what reads the diff.
- Walking the stack. A branch of eight commits becomes one account rather than eight paragraphs.
- Counting. No file, line, test, or commit counts.
- Provenance filler. Nothing about requests, review rounds, sessions, prompts, models, or tools.
- Markdown, meaning fenced blocks, headings, emphasis, links, or tables. Backticks around a literal identifier are fine.

Hard wrap the body at 72 characters and the trailers at 100.

### Footer

The script fills this in. The trailers come from the commits themselves, ordered with attribution first and sign-off last. `Closes` lines come from the description's Related section, and only for a number GitHub reports as an open issue. A pull request, a closed issue, or a number GitHub wouldn't describe gets a `# Not closed:` line printed under the skeleton instead, outside the message.

Read those lines, then check the `Closes` lines that remain against the branch. An open issue listed under Related isn't always one this branch closes, and a closing keyword acts on merge without asking.

## Step 4: run the commit-msg gates

Fix the prose findings with the `fix-prose` skill first, passing the draft and the task that judges it:

```text
Skill(fix-prose, args: "SQUASH_AGENTMSG mise run lint-squash-msg")
```

Then run the task yourself:

```text
mise run lint-squash-msg
```

The same four hooks a commit must pass, plus the source-range identity context that requires `Assisted-by` when the squash collapses an agent commit:

- vale under the commit scope, which catches AI tells through `ai-tells-commits`
- cspell with the commit dictionary
- commitlint for the Conventional Commits shape
- commit-trailers for trailer order

Order matters here. The review in the next step stamps the exact bytes it read, so a lint fix after it voids the stamp and costs another review round over a comma. The merge script runs this task again before it merges, and refuses when the task is absent rather than skipping it.

## Step 5: review the message

```text
bash .claude/skills/merge-pr/scripts/review.sh <number>
```

Pass `--note <file>` to include a decision the operator actually made, with the operator's reason. Never use it for the agent's own preference.

The script snapshots the draft and the pull request, then starts a fresh, read-only reviewer as a headless process of the harness's own CLI, with the `review-squash-message` skill as its instructions. That reviewer hasn't watched you work. It reads the message against every commit the squash collapses, and its first question is the one you can no longer ask yourself by this point: whether the message left out anything that mattered in those commits. The script reads the verdict itself. On a clean verdict it writes a stamp binding the draft's exact bytes, the pull request, its head, its base branch, and the commit that base pointed at. Findings remove any earlier stamp, and so does editing the draft, a push to the branch, or a move of the base.

This step is mandatory, and the merge script enforces it.

| Exit | Meaning |
| --- | --- |
| 0 | Passed and stamped |
| 75 | Waiting for an approving review GitHub requires |
| 2 | Changes required: the output lists the findings |
| 4 | The reviewer failed or replied without a readable verdict |
| 124 | The reviewer timed out without a stamp |
| 5 | The draft or the pull request moved while the review ran |

Fix everything it returns and run it again. Push back only where it's demonstrably wrong about the commits, and say why.

A finding cites one instance rather than the fault itself. Fix the instance, then read the rest of the message for the same fault.

`DEVTOOLS_REVIEW_CMD`, `DEVTOOLS_REVIEW_MODEL`, and `DEVTOOLS_REVIEW_EFFORT`, or the same keys in `~/.config/devtools/<repository>.conf`, replace the default reviewer. No repository file can, so a branch never chooses who reviews its own merge.

## Step 6: confirm with the operator

`AskUserQuestion` truncates its options. The operator has to read the message in your message text rather than in the widget. Print this first, verbatim:

```text
Merging: #<number> <title>
Into:    <base branch>
Commits: <count> collapsing into one

<the entire SQUASH_AGENTMSG contents, verbatim>
```

The merge script settles who confirmed the merge, in this order, and prints which one it took:

1. `--confirmed`, which you pass only after the operator answered yes to this message in this session
2. a merge pre-approval for this session, which the operator granted through `mise run preapprove merge` and preflight reported as `merge: GRANTED`
3. the machine's delegation check, when preflight reported one configured: it answers for the repository and the action `merge`, and a refusal stops the merge

A bare `mise run preapprove` grants commit and pr and never merge, because a merge is the one action here no later step can undo. With none of them the script exits 75 and merges nothing. Ask the operator, then run it again with `--confirmed`.

A grant or a delegation answers one question, merge under this message. Ask the operator anyway where any of these holds:

- the review returned a finding nobody acted on
- the checks aren't green
- this pull request isn't the one the session set out to merge, which covers a dependency bump nobody here authored unless the operator named it

Where preflight reported `merge: GRANTED` or a delegation check, print the preceding block so the operator still sees the message, state which one this merge goes under, and move to step 7 without calling `AskUserQuestion`.

Otherwise call `AskUserQuestion` with `question` set to `Squash merge with this message?`, `header` set to `Merge`, `multiSelect` set to `false`, and these four options in order:

| Label | Description |
| --- | --- |
| `Merge it` | The subject line, verbatim |
| `Revise the message` | `Redraft the message, review it again, and come back.` |
| `Wait on the checks` | `Hand this to watch-pr first, and merge once it reports green.` |
| `Stop here` | `Leave the pull request, the draft, and the branch untouched.` |

Follow the answer. `Merge it` is what `--confirmed` stands for in step 7. Revising sends you back through the review, because the stamp binds bytes.

## Step 7: merge and clean up

```text
bash .claude/skills/merge-pr/scripts/squash-merge.sh [--confirmed] <number>
```

Pass `--confirmed` only when the operator chose `Merge it` in step 6. Merge only through this script. The guard hook refuses a `gh pr merge` you write out yourself, because a direct call skips every gate below and nothing lints a squash commit afterwards.

That script runs every gate again before it touches anything. It checks that the message references this pull request, and that the stamp covers the bytes on disk, the live head, the base branch, and the commit that base points at. Where the configuration specifies `gh_login`, the GitHub CLI has to authenticate as that account. Otherwise, the script says it skipped the check. The pull request has to be open, ready, mergeable with a merge state of `CLEAN`, approved where the repository requires a review, and green by the latest run of every check. GitHub reports whether a pull request can merge as `UNKNOWN` for a few seconds after the base moves, so the second of two merges in a row meets it in most cases. The script waits that out for up to a minute before judging, and refuses `CONFLICTING` at once. Then the linters run, and then the confirmation resolves.

The merge specifies the reviewed head, so a commit pushed while the final checks run makes GitHub refuse the merge rather than squash it in. Then the script removes `SQUASH_AGENTMSG` and `PR_AGENTDESC.md` along with their stamps, and deletes the merged branch only when it belongs to this repository. It leaves a fork's branch untouched.

Cleanup falls to the script that watched the merge succeed, because there's no post-merge hook to run it from.

Report the merged commit. Where this worktree stands on the branch that merged, the script says so rather than deleting the branch out from under the session. The operator decides whether to switch or drop the worktree.

## Preconditions

- `gh` installed and authenticated, and `jq`
- a `mise run lint-squash-msg` task
- a gitignore entry for `SQUASH_AGENTMSG`
- the `review-squash-message` skill deployed alongside this one

Preflight checks each. Where one is missing, tell the operator rather than improvising a substitute.

## API quota

Identity and explicit readers use GraphQL, which doesn't spend REST core quota. Each checks snapshot uses one GraphQL call. Quota failures return 76 and print the resource's reset time. Other API failures keep GitHub's error. The watcher returns 2 for ordinary check-read failures. A missing reset header needs one REST `/rate_limit` lookup, which doesn't consume core quota. GitHub CLI writes can still spend REST quota resolving or applying labels, milestones, reviewers, and assignees. Scripts never retry writes automatically.
