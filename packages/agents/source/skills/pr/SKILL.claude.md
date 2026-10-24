---
name: pr
license: Apache-2.0
description: >-
  Open a pull request for the current branch. The session drafts the title and the pull request properties and the template's sections into PR_AGENTDESC.md. An independent reviewer then reads that draft against the branch, and a mechanical validator checks it against the template. The operator confirms before anything publishes. Whatever follows routes to the watch-pr and fix-pr and merge-pr skills. Use this whenever the user asks to open or draft a pull request in a tbhb repo, and whenever a task ends in one.
hooks:
  PreToolUse:
    - matcher: Bash
      hooks:
        - type: command
          command: "${CLAUDE_PROJECT_DIR}/.claude/skills/pr/scripts/guard-gh.sh"
---

# Open a pull request

Use `gh-post` for issue and PR content, comments, reviews, comment edits, and release notes. Supply body text through `--body-file` or stdin. If `gh-post` is missing, stop and report the missing command. Never fall back to a direct GitHub content write.

Work the steps in order. Scripts enforce every gate here. Step 4's review script stamps what an independent reviewer cleared. Step 7's publish script refuses a draft, branch, or base the stamp doesn't cover, a push that would rewrite origin without the operator's word, and a publish nobody confirmed. Both hold in any session, including a subagent's.

A guard hook refuses every `gh pr` verb that changes a pull request outside the publish script.

This skill ends when the pull request is open. From there `watch-pr`, `fix-pr`, and `merge-pr` take over, and step 8 routes to whichever the operator asked for.

## Preflight

!`bash ${CLAUDE_SKILL_DIR}/scripts/preflight.sh`

## Step 0: the checklist

Track these steps with the session's task-list tools where it provides them. Newer harnesses leave those tools out by default, and a session without them works the list in order as written.

1. Confirm the branch is ready
2. Draft the description in PR_AGENTDESC.md
3. Pass the prose gates with fix-prose
4. Review the draft with the review script
5. Run the validator and the prose gates
6. Ask the operator how far to take it
7. Publish the pull request
8. Route the rest

Stop before any of it where preflight reports a rebase, merge, or cherry-pick in progress, or a missing precondition. Say what's wrong and hand back.

@include parts/ready.md

## Step 2: draft the description

Read the branch diff against the resolved base from preflight, the commits that would land, and the repository's pull request template. Write or revise `PR_AGENTDESC.md` in this session. Fill the scaffold's frontmatter properties, a Conventional Commits title describing the whole branch, and every template section. Use the labels and properties preflight reported, and describe only verification you actually ran.

The `write-pr-description` skill remains an optional helper when delegating the drafting is useful. Publication requires the stamp from step 4's scripted review for either drafting path.

## Step 3: pass the prose gates

Invoke the `fix-prose` skill, passing the draft and the task that judges it:

```text
Skill(fix-prose, args: "PR_AGENTDESC.md mise run lint-pr-description")
```

It runs the lint rounds in a subagent, which keeps the findings out of this session.

Run it before the review rather than after. The review in the next step stamps the exact bytes it cleared, and a lint fix landing later voids that stamp and buys another round.

## Step 4: review the draft, and loop

```text
bash .claude/skills/pr/scripts/review.sh
```

Pass `--note <file>` to include a decision the operator actually made, with the operator's reason. Never use it for the agent's own preference.

@include parts/review.md

@include parts/checks.md

## Step 6: ask the operator how far to take it

Print the draft first so the operator reads it in your message text rather than in the truncated widget:

```text
Branch:  <branch> into <base>
Commits: <count>

<the entire PR_AGENTDESC.md contents, verbatim>
```

@include parts/confirm.md

Where preflight reported `pr: GRANTED` or a delegation check, print the preceding block so the operator still sees the draft, state which one this publish goes under, and move to step 7 without calling `AskUserQuestion`.

Otherwise call `AskUserQuestion` with `question` set to `How far should I take this?`, `header` set to `Pull request`, `multiSelect` set to `false`, and these four options in order, from the most automated to the least:

| Label | Description |
| --- | --- |
| `Open, fix, and merge` | `Open it, watch the checks, fix what fails, and squash merge once it is green.` |
| `Open, watch, and fix` | `Open it, watch the checks, and fix what fails. Stop before merging.` |
| `Open and watch` | `Open it, watch the checks, and report the result without changing anything.` |
| `Open only` | `Open it and hand back.` |

Ask a second question in the same call. Set `question` to `How should fix commits land?`, `header` to `Fixes`, `multiSelect` to `false`, and give these two options:

| Label | Description |
| --- | --- |
| `Separate commits` | `Each fix lands as its own commit. History keeps the record of what broke.` |
| `Amend and force-push` | `Fixes fold into the commit that caused them, pushed with --force-with-lease.` |

Neither answer is the safe default in general. A separate commit suits a branch under review, where a reviewer needs to see what changed since they last looked. Amending suits a branch nobody has read yet, where a lint fix of its own is noise the squash message would have to account for.

Any of the four first answers is what `--confirmed` stands for in step 7. Record both answers. Step 8 routes on the first and passes the second to `fix-pr` in its arguments, so nobody asks either one again. Where the operator wants the description changed instead, redraft it, put it back through the review, and return here.

## Step 7: publish the pull request

After the operator agrees to rewrite the remote branch, these personal `permissions.allow` rules let Claude Code auto mode run the exact supported forms:

```text
Bash(bash .claude/skills/pr/scripts/create-pr.sh --rewrite)
Bash(bash .claude/skills/pr/scripts/create-pr.sh --confirmed --rewrite)
```

The operator adds them to their Claude Code settings. The agent never changes its own permissions.

```text
bash .claude/skills/pr/scripts/create-pr.sh [--confirmed] [--rewrite]
```

Pass `--confirmed` only when the operator answered step 6 in this session, and `--rewrite` only when the operator agreed to replace the branch on origin after a rebase.

@include parts/publish.md

Never call `gh pr create` yourself. A guard hook refuses it, because that script runs every gate named here. Report the URL.

@include parts/route.md

@include parts/preconditions.md

## API quota

Identity and explicit readers use GraphQL, which doesn't spend REST core quota. Each checks snapshot uses one GraphQL call. Quota failures return 76. The printed message says which limit applied and what to do next: wait at least a minute after a secondary limit, or read the budget with a GraphQL `rateLimit { remaining resetAt }` query once a primary GraphQL limit has passed. GitHub doesn't report the reset time with the failure. No script looks it up through `/rate_limit`. The original error follows unchanged. Other API failures keep GitHub's error. The watcher returns 2 for ordinary check-read failures. GitHub CLI writes can still spend REST quota resolving or applying labels, milestones, reviewers, and assignees. Scripts never retry writes automatically.
