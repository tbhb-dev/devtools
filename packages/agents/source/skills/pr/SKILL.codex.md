---
name: pr
description: >-
  Open or update a pull request for the current branch. The session drafts PR_AGENTDESC.md, an independent headless reviewer reads it against the branch, and a mechanical validator checks it against the template. The operator confirms before anything publishes. Use whenever the user asks to open or draft a pull request in a tbhb repository, or when a completed task should end in one. Routes later CI work to watch-pr, fix-pr, and merge-pr.
---

# Open a pull request

Use `gh-post` for issue and PR content, comments, reviews, comment edits, and release notes. Supply body text through `--body-file` or stdin. If `gh-post` is missing, stop and report the missing command. Never fall back to a direct GitHub content write.

Work the steps in order. Scripts enforce every gate here. Step 4's review script stamps what an independent reviewer cleared. Step 7's publish script refuses a draft, branch, or base the stamp doesn't cover, a push that would rewrite origin without the operator's word, and a publish nobody confirmed. Codex doesn't provide a skill-scoped command hook, so those scripts are the whole enforcement. Never run a `gh pr` command that changes a pull request yourself.

## Preflight

Run this from the repository holding the branch before anything else:

```text
bash .agents/skills/pr/scripts/preflight.sh
```

## Step 0: the checklist

Where the harness provides `update_plan`, track these with it, keeping exactly one item `in_progress` at a time. Otherwise work the list in order as written.

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

Run the `fix-prose` skill on `PR_AGENTDESC.md mise run lint-pr-description` where it's available. Otherwise run that task directly and resolve only mechanical findings. Run it before the review: the review stamps the exact bytes it cleared, and a later lint fix voids that stamp.

## Step 4: review the draft, and loop

```text
bash .agents/skills/pr/scripts/review.sh
```

Don't delegate the review to an agent of your own, and don't write a verdict file. The reviewer is a `codex exec` child in a read-only sandbox and needs the network. Escalate the script's permissions where this session's sandbox blocks it.

Pass `--note <file>` to include a decision the operator actually made, with the operator's reason. Never use it for the agent's own preference.

@include parts/review.md

@include parts/checks.md

## Step 6: ask the operator how far to take it

Show the operator the branch and the complete draft first:

```text
Branch:  <branch> into <base>
Commits: <count>

<the entire PR_AGENTDESC.md contents, verbatim>
```

@include parts/confirm.md

Where preflight reported `pr: GRANTED` or a delegation check, state which one this publish goes under and move to step 7 without `--confirmed`.

Otherwise end the turn and ask `How far should I take this?`, offering `Open, fix, and merge`, `Open, watch, and fix`, `Open and watch`, and `Open only`. Also ask whether fixes use separate commits or an amend and force-push. Don't publish in the turn that asks. The original request to open a pull request doesn't count as this confirmation. Any of the four answers is what `--confirmed` stands for in step 7.

## Step 7: publish the pull request

```text
bash .agents/skills/pr/scripts/create-pr.sh [--confirmed] [--rewrite]
```

Pass `--confirmed` only when the operator answered step 6, and `--rewrite` only when the operator agreed to replace the branch on origin after a rebase.

@include parts/publish.md

Report the URL.

@include parts/route.md

@include parts/preconditions.md

## API quota

Identity and explicit readers use GraphQL, which doesn't spend REST core quota. Each checks snapshot uses one GraphQL call. Quota failures return 76. The printed message says which limit applied and what to do next: wait at least a minute after a secondary limit, or read the budget with a GraphQL `rateLimit { remaining resetAt }` query once a primary GraphQL limit has passed. GitHub doesn't report the reset time with the failure. No script looks it up through `/rate_limit`. The original error follows unchanged. Other API failures keep GitHub's error. The watcher returns 2 for ordinary check-read failures. GitHub CLI writes can still spend REST quota resolving or applying labels, milestones, reviewers, and assignees. Scripts never retry writes automatically.
