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

## Step 1: confirm the branch is ready

Preflight answered each of these, so read rather than re-run:

- The branch has commits the base doesn't. Nothing to open otherwise.
- The branch isn't the default branch.
- Uncommitted changes stay out of the pull request. Where preflight listed some, say so and let the operator decide before going on.
- A pull request may already exist. Where one is open, publishing updates it rather than opening a second.
- The branch has a current base. Where preflight reports it behind the default branch, run the `rebase` skill before opening. The reviewer then reads the branch against the base it actually merges into.

Preflight also settled the draft on disk. It removed a stale `PR_AGENTDESC.md` where no open pull request matched it, then scaffolded a fresh one from the template. That scaffold has the frontmatter keys, a placeholder title, and every section the template declares, so nothing downstream reproduces the template's shape from memory. It fails the validator until something fills it, deliberately.

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

The script snapshots the draft, `HEAD`, the base the draft names, and the merge base, then starts a fresh, read-only reviewer as a headless process of the harness's own CLI, with the `review-pr-description` skill as its instructions.

It resolves the base to `origin/<base>` where origin has a copy, the same commit preflight and the writer compared against, so a stale local branch never adds merged commits to what the reviewer reads. The reviewer watched neither the work nor the drafting, and the script reads its verdict itself.

| Exit | Meaning |
| --- | --- |
| 0 | Passed and stamped |
| 2 | Changes required: the output lists the findings |
| 3 | The reviewer rejected the same draft for this branch head. Change it or ask the operator |
| 4 | The reviewer failed or replied without a readable verdict |
| 124 | The reviewer timed out without a stamp |
| 5 | The draft, `HEAD`, or the base moved while the review ran |

On findings, revise the draft in this session or pass them verbatim to the optional `write-pr-description` helper, then run the review again. Bound it at three rounds, counting only the rounds a finding caused. A round the branch caused doesn't count against the bound: a remediation commit or a rebase changes what the description has to describe. Say which kind each round was as you go, so the count is accurate. Past three, stop, report the count with the findings that keep coming back, and let the operator decide whether to go on.

The script records every readable verdict with the draft digest and branch head. After `CHANGES REQUIRED`, it refuses to send byte-identical text for the same head to another reviewer. The reviewer can read a changed draft.

This step is mandatory, and `create-pr.sh` enforces it.

A clean verdict writes a stamp binding the draft's exact bytes, `HEAD`, the resolved base ref, that ref's commit, and the merge base. Findings remove any earlier stamp. Editing the draft, a new commit, or a fetch that moves the base voids it, and the publish refuses until the review runs again.

`review_cmd` chooses the reviewer. It isn't only a label. From Codex, configure a reviewer that the session may start. These repository machines use `agy` in read-only plan mode through this line in `~/.config/devtools/devtools.conf`:

```text
review_cmd = agy -p "$(cat)" --mode plan --add-dir "$DEVTOOLS_REVIEW_INPUTS" --print-timeout 900s
```

`DEVTOOLS_REVIEW_CMD`, `DEVTOOLS_REVIEW_MODEL`, and `DEVTOOLS_REVIEW_EFFORT`, or the same keys in `~/.config/devtools/<repository>.conf`, replace the default reviewer. No repository file can.

## Step 5: run the validator and the prose gates

```text
mise run lint-pr-description
```

That task runs the mechanical checks, then vale and cspell over the draft. The validator settles the frontmatter structure, the title's form and bounds, section presence and order, empty sections, surviving comments, unclosed fences, dead links, and whether every backticked path exists. It reads the title's type and the paths against origin's copy of the base, as the review does. Each finding gives a line and the fix.

Resolve every one through `fix-prose` rather than editing it yourself, because a direct edit here spends the context that skill exists to save. Edited the draft either way? Then step 4 runs again, because the stamp binds bytes rather than intentions.

## Step 6: ask the operator how far to take it

Print the draft first so the operator reads it in your message text rather than in the truncated widget:

```text
Branch:  <branch> into <base>
Commits: <count>

<the entire PR_AGENTDESC.md contents, verbatim>
```

`create-pr.sh` settles who confirmed the publish, in this order, and prints which one it took:

1. `--confirmed`, which you pass only after the operator answered this step's question in this session
2. a pre-approval for this session, which the operator granted through `mise run preapprove` and preflight reported as `pr: GRANTED`
3. the machine's delegation check, when preflight reported one configured: it answers for the repository and the action `pr`, and a refusal stops the publish

With none of them the script exits 75 and publishes nothing. Ask the operator, then run it again with `--confirmed`.

Where preflight reported `pr: GRANTED`, take the answers from the grant rather than from a question:

| Preflight line | How far to take it | How fixes land |
| --- | --- | --- |
| `pr: GRANTED`, `merge: GRANTED` | `Open, fix, and merge` | Separate commits |
| `pr: GRANTED`, `merge: not granted` | `Open, watch, and fix` | Separate commits |

Separate commits are the pre-approved choice because amending rewrites a commit the operator already cleared, and a grant covering that would go beyond the question it answers. Amend only where the operator asks for it in the session.

A grant or a delegation answers one question, publish this description, and says nothing about whether the branch is right. Ask the operator anyway where any of these holds:

- the review returned a finding nobody acted on
- a gate needed more than a mechanical fix
- the branch goes beyond what the session set out to do
- the push would rewrite the branch on origin

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

That script is the only thing here that publishes, and it gates itself first:

- runs the validator over the draft again
- checks the review stamp against the draft's bytes, `HEAD`, the resolved base, and the merge base, and takes its own copy of the stamped bytes to publish from
- resolves labels, the milestone, and every issue reference against the API, and refuses cleanly on an unknown one rather than leaving a published pull request missing what it should contain
- resolves the confirmation, then checks the stamp again
- checks that `gh` uses the machine's configured `gh_login` for its signed-in account before any write, and says so in its output where the machine configures none

The push is a fast-forward of what origin holds, read from origin itself. A rebase after the last push requires a rewrite. The plain run refuses it. `--rewrite` replaces only the exact commit origin holds, leasing on it. That flag needs the operator's word, and it still refuses where `HEAD` is behind origin or where origin holds a commit this checkout never fetched. Where the operator would rather not rewrite, merge the base into the branch instead. `--dry-run` runs every check and says what the push would do. It warns when the real run would meet a non-fast-forward. It confirms and publishes nothing.

After the push it checks that `HEAD` is still the reviewed commit and that origin holds it. Then it opens the pull request, or brings the open one up to date: title, body, base, labels, reviewers, assignees, milestone, and draft state all converge on the draft, except an existing owner-review label stays in place. Drafts must omit the label named by `owner_review_label`, which defaults to `owner-review`. Only the owner-approval route applies it after approval at the current head, green checks, and a mergeable state. A second run updates rather than opening another.

Use `--dry-run` to check without writing and `--rerun-gates` to force fresh gates. The script can skip checks while the cache key is unchanged: the clean committed tree, command, gate inputs, and tool versions must match. Description checks also bind the draft. It prints each check it skips. Leak scans and the scripted review always run.

Use `--check-description` to run the description validator and prose lint along with labels and property checks. This skips scans, review, lint, tests, and publication. Publication runs description checks, leak scans, lint, the scripted review, and tests in that order.

Never call `gh pr create` yourself. A guard hook refuses it, because that script runs every gate named here. Report the URL.

## Step 8: route the rest

Follow the answer from step 6.

`Open only` ends here. Say the pull request is open and hand back.

`Open and watch` invokes `watch-pr` with the number, then reports what it found. Stop there even when something failed.

`Open, watch, and fix` invokes `watch-pr`, then `fix-pr` on failure, then `watch-pr` again to confirm. Repeat until the checks pass, then stop before merging. Bound the loop. After three rounds on the same check, stop, then report what each attempt changed.

`Open, fix, and merge` does the same, then invokes `merge-pr` with the number once the checks pass. That skill drafts the squash message and puts it through its own review. It also asks the operator to confirm before merging, which gives the merge a confirmation of its own.

Where the description drifts as remediation commits arrive, redraft it, review it again, and re-run `create-pr.sh` to update the published copy.

## Preconditions

This skill assumes the shared tbhb toolchain:

- `gh` installed and authenticated
- a `mise run lint-pr-description` task
- a gitignore entry for `PR_AGENTDESC.md`
- a pull request template at `.github/pull_request_template.md`
- the `review-pr-description` and `fix-prose` skills deployed alongside this one
- the `watch-pr`, `fix-pr`, and `merge-pr` skills deployed for step 8

Preflight checks each. Where one is missing, tell the operator rather than improvising a substitute.

## API quota

Identity and explicit readers use GraphQL, which doesn't spend REST core quota. Each checks snapshot uses one GraphQL call. Quota failures return 76 and print the resource's reset time. Other API failures keep GitHub's error. The watcher returns 2 for ordinary check-read failures. A missing reset header needs one REST `/rate_limit` lookup, which doesn't consume core quota. GitHub CLI writes can still spend REST quota resolving or applying labels, milestones, reviewers, and assignees. Scripts never retry writes automatically.
