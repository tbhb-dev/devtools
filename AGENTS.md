# Agent instructions

Guidance for AI coding agents working in this repository. Read it alongside the per-tool documentation and any memory files the harness loads.

Follow the [N-2 compatibility policy](docs/compatibility.md) when changing consumer-facing behavior.

## Commit messages

Run the `commit` skill. It runs the whole sequence, from choosing the rebase base through the commit and the rebase that follows, and this section only summarizes it.

Draft every commit message in the repo-root file `COMMIT_AGENTMSG` before you commit. A gitignore entry keeps that file out of history. Because the post-commit hook deletes it after the commit succeeds, each commit starts from a blank scratchpad.

1. Group the outstanding work into atomic commits, and stage the paths for one of them by name. Never `git add -A` or `git add .`.
2. Write the full message (subject, body, and trailers) to `COMMIT_AGENTMSG`. The body explains why the change exists, because the diff already says what changed.
3. Review the draft with the skill's `review.sh`, which starts the `review-commit-message` reviewer as an independent headless process and stamps what it cleared.
4. Run `mise run lint-commit-msg` and resolve whatever it reports.
5. Confirm the message with the operator, then commit the validated draft through the skill's `commit.sh`.

`mise run lint-commit-msg` previews the commit-msg hook against a temporary copy of the draft. When no sign-off exists it appends the optional machine `signoff_identity`, falling back to `git config user.name` and `user.email`, only if the configured identity rules permit it. Set `signoff_identity` when the Git identity is an agent so the responsible person supplies the DCO attestation. It preserves an existing sign-off and leaves the draft unchanged. The real hook still requires the trailer. See [the sign-off preview](docs/commit-signoff-preview.md) for identity and existing-trailer behavior. Agent drafts still include the complete trailers before review.

The preview runs:

- vale under the commit scope, which catches AI commit tells via `ai-tells-commits`
- cspell with the commit dictionary
- commitlint for the Conventional Commits shape
- commit-trailers for trailer order

An agent-written commit always requires `Assisted-by`, even when a harness reminder says to omit attribution. That setting controls the harness's own `Co-authored-by` lines, not this repository's trailer policy.

Running it while drafting reports problems early, rather than at the commit-msg hook where a late failure interrupts the commit.

The prek commit-msg hook on `.git/COMMIT_EDITMSG` stays the real gate. `COMMIT_AGENTMSG` and its recipe only preview that gate, so a clean recipe run predicts a clean commit but never replaces the hook.

That script is the only thing here that commits. Before committing it checks the review stamp and settles who confirmed the commit. Afterwards it reads the commit back and prints any staged path missing from it. prek stashes and restores the worktree around the pre-commit hooks, so a failed attempt can leave a path unstaged that you staged before it, and without that comparison the retry commits part of the group and doesn't report the omission. It reads the landed paths against the first parent, so a merge commit and a repository's first commit both compare correctly. It also fails when the commit's tree differs from the tree the reviewer read, and leaves the commit in place for a person to compare. Only a hook that rewrites the index and still lets the commit through can cause that. A hook that fixes a file in place fails the commit instead, and staging its fix moves the tree, which sends the change back through review.

The review is script-owned, so it holds in any session, a subagent's included. `review.sh` snapshots the draft, the staged tree, and `HEAD` under `<git dir>/agent-skills/`, then runs the harness's own CLI headless and read-only: `claude -p` limited to Read, Grep, and Glob, or `codex exec` in its read-only sandbox. The `review-commit-message` skill supplies the criteria. The script parses the verdict strictly, requires that nothing in its snapshot moved while the review ran, and only then writes `commit.stamp`, binding the draft's digest, the tree, `HEAD`, and the mode. Editing the draft, restaging, or moving `HEAD` after the review means running it again. `DEVTOOLS_REVIEW_CMD`, `DEVTOOLS_REVIEW_MODEL`, and `DEVTOOLS_REVIEW_EFFORT` replace the default reviewer, from the environment or `~/.config/devtools/<repository>.conf` and never from a file in the repository, because a commit must not be able to choose who reviews the next one. The stamp is an honesty boundary rather than a lock: anything running as the same user can write it by hand, and doing so shows in a transcript.

Under Claude the skill's frontmatter also declares a guard hook. It's a rail rather than the gate, and it applies only inside a commit workflow: preflight records the commit `HEAD` sits on, and while `HEAD` is still there the guard refuses whole-tree staging and any `git commit` written out by hand. After the commit that ends the workflow moves `HEAD` past that mark it refuses nothing. Every check that matters is in the scripts instead, because frontmatter hooks don't fire inside a subagent and Codex has no such hook at all.

## Unattended commits

Run `agent-commit` for delegated work that must finish without a confirmation prompt. This separate generated skill for both harnesses calls the same commit scripts, shared library, gates, reviewer, and stamp machinery as `commit`. It refuses `--confirmed`: only a session pre-approval or the machine's delegation check grants the write. Missing authority returns 75 without asking a question.

The skill requires SSH signing and the repository's executable hooks. Verification checks the resulting message, tree, parents, branch, and signature. Signing failure holds the change. A verification failure retains the commit and review evidence for recovery rather than moving a branch that a hook may already have pushed. Review findings have a persistent bound, three rounds by default. Reaching it returns 3 and requires operator inspection. A successful commit clears that counter.

Repository behavior comes from `.agents/agent-skills.conf`, outside the deployed skills. The reader accepts declarative `key = value` lines and never sources the file. It rejects unknown and duplicate keys. Supported settings are `lint_commit_cmd`, `required_hooks`, `hook_marker`, and `review_max_rounds`. Reviewer selection and delegation still come only from machine configuration. The review stamp binds repository configuration as well as the draft and Git state.

The [consumer migration note](docs/agent-skills-migration.md) covers settings, local files to remove, and the remaining commit policy work. The package includes `agent-commit`, `agent-pr`, and `agent-merge-pr`.

## Unattended pull requests

Run `agent-pr` to publish a committed agent branch without a confirmation prompt. Its generated entry points call the same PR validator, gates, review, stamp, and publisher as `pr`. Only session pre-approval or machine delegation grants `pr`, and machine `gh_login` must match the active account before the push and again before the GitHub write. It refuses dirty trees, interrupted operations, detached `HEAD`, and `--confirmed`. Review findings use the persistent bound shared with `agent-commit`. Preflight preserves unpublished drafts and their bounds.

Repository config also accepts `pr_template`, `lint_pr_cmd`, `pr_log_entry`, `pr_related_pattern`, `pr_title_check_cmd`, and `owner_paths`. Scaffolding, validation, and the review snapshot use the same template. Owner paths supply a restrictive `--owner-path` signal to the machine delegation checker for any matching changed path across the branch. This includes deletions and renames. The checker still supplies the authority.

Publication records the PR number, URL, full head, base, draft state, and push kind under the worktree's Git directory. Reports print locally unless `mail_cmd` and `mail_to` configure a transport, with optional `mail_from`. A reporting failure prints the unsent report without repeating publication. The migration note defines the command interfaces and what consumers can remove.

## Unattended merges

Run `agent-merge-pr` for unattended squash merges. Its generated entry points use the shared merge scripts, independent review, gates, stamps, and confirmation chain. Machine `gh_login` and `required_reviewer` are mandatory. The required reviewer must approve the exact current head. Configure its GraphQL login without the REST `[bot]` suffix. Optional machine `required_reviewer_type` (`Bot` or `User`) checks the review author type. Leaving it unset preserves login-only matching. A GitHub App installation needs Contents read and write permission for its approval to count, as [#300](https://github.com/tbhb-dev/devtools/issues/300) documents. Only session pre-approval or machine delegation grants `merge`. The skill refuses `--confirmed`.

A `merge-pr` execution without an explicit `--confirmed` runs the same approval, owner, policy, and required-check gates, including when confirmation comes from session pre-approval or machine delegation. Only an operator answer represented by `--confirmed` keeps the attended gate behavior. An empty check rollup is inconclusive and refuses every merge.

Generated unattended entry points use `DEVTOOLS_UNATTENDED` with `DT_UNATTENDED` as a deprecated alias for one release, and `DEVTOOLS_UNATTENDED` takes precedence when set.

Repository configuration supplies squash lint, readiness, ruleset location, owner-review label, protected paths, suppression patterns, and disposable ignored paths. Check judgement calls `agentrepo checks evaluate`. Required checks come from applied GitHub rules and ruleset files at the pinned base. No checks or known requirements means no unattended merge. The YAML policy reader requires `uv`.

devtools keeps its merge patterns in `.agents/agent-skills.conf`. They protect the published payload, workflows, rule sets, gate definitions, and skill guards. Cleanup permits build and cache output and preserves local settings and agent worktrees. Policy errors include the refusal reason and specify any empty pattern.

Golden files and detected gate loosening require an exact-head APPROVED review by machine `merge_owner`, also available as `DEVTOOLS_MERGE_OWNER`. Use the GraphQL login without `[bot]` for `merge_owner` too. Keep this authority a person: an App owner override can approve its own golden-file waivers. Repositories can't choose this authority, and pushing invalidates approval.

When using an override, the merge report specifies the approving login. It doesn't waive the owner-review label, required checks, independent review, or write authority.

From the base commit's lockfile, the scan reads non-local APM deployments and preserves local overrides. It skips comments and quoted marker text and matches exclusion keys. Policy configuration changes require owner approval. After delegation and changing dependent pull requests' base branches, the merger checks live approvals. It pins the merge to the reviewed head and verifies the remote message before cleanup. Cleanup preserves fork branches, newer work, current worktrees, and valuable ignored files. The migration note for consumers defines settings and remaining commit work.

## GitHub content posts

Use `gh-post` for issue and PR content, comments, reviews, comment edits, and release notes. Supply body text through `--body-file` or stdin. The command refuses inline bodies and unknown flags. If the command is missing, report `gh-post` as missing and stop before posting. Never fall back to a direct content write.

Machine configuration supplies allowed posting identities and private strings for the target repository. Findings print category and line only, and successful posts record a body hash in the local audit log. See [the posting guide](docs/gh-post.md) for installation, configuration, and exit codes.

## Pull requests

Preflight and publication refuse more than 800 source lines of code or 2,000 prose lines against the draft's publish base. Split by concern, along commit boundaries, or as a stacked series. Only Renovate bot authors (`app/renovate` or `renovate[bot]`, with `is_bot`) are exempt. The checks measure `chore` titles and other bots. The skills require `agentrepo` v0.23.0 or newer. That release adds `size check`. Install it and the pinned `scc` before adopting these skills. Publication stops until you install that version or newer. The size limit has no override.

Publication checks the description before leak scans, lint, the independent review, and tests. Use `create-pr.sh --check-description` to run only description checks. PR and commit workflows can use gate passes from a clean committed tree while the cache key is unchanged: the tree, command, gate definitions, and tool versions must match. Description gates also bind the draft. Records stay under the Git directory. The scripts print each cached pass they use, and `--rerun-gates` forces fresh checks. Leak scans and independent reviews always run.

Run the `pr` skill. It drafts the description and publishes it after a review, and it routes what follows to three sibling skills. This section only summarizes them.

Draft every pull request description in the repo-root file `PR_AGENTDESC.md`, which a gitignore entry keeps out of history. The post-commit hook deletes the commit draft. This one remains after the publish as the working copy of the description while the pull request is open, and `merge-pr` removes it once the branch merges. A leftover draft that matches no open pull request has gone stale, so the `pr` preflight removes it rather than letting the next run inherit abandoned text.

The file supplies three things the template can't. YAML frontmatter holds the pull request properties, a level 1 heading holds the title, and the rest fills in the sections from `.github/pull_request_template.md`.

1. Draft the description in the session running `pr`, with a title in the Conventional Commits shape. A squash merge turns that title into the commit subject on the default branch.
2. Review the draft through `.claude/skills/pr/scripts/review.sh`, which runs a fresh read-only reviewer with `review-pr-description` as its criteria.
3. Run `mise run lint-pr-description` and resolve whatever it reports.
4. Confirm with the operator, then publish through `.claude/skills/pr/scripts/create-pr.sh`.

Review stamps bind the draft's bytes, `HEAD`, the base resolved to `origin/<base>` where origin has one, that ref's commit, and the merge base. Both the validator and the reviewer compare against that same resolved base, so a stale local default branch never counts merged commits as landing. `create-pr.sh` checks every binding before and after resolving the confirmation (`--confirmed`, then a pre-approval, then the machine's delegation check). Before any write, the active `gh` account must match the machine's `gh_login`, and the output says when the machine has no configured login. Pushing requires a fast-forward from the commit read through `git ls-remote`. After a rebase, a branch goes up only with `--rewrite`, which leases on that exact remote commit and refuses when `HEAD` is behind origin or origin holds a commit never fetched here. `--dry-run` warns before the real run meets a non-fast-forward.

`mise run lint-pr-description` runs a mechanical validator plus vale and cspell. The validator is offline and settles only what looking can settle:

- the frontmatter structure, its known keys, and a label the pull request actually has
- the title's Conventional Commits form, its bounds, and whether its type appears among the commits that would land
- the template's sections, their order, and any left empty
- instructional comments that survived, unclosed fences, and links pointing nowhere
- whether every path the description puts in backticks exists in the tree or the branch diff

Each finding states a line and the fix. Resolving one means opening the draft at that point rather than searching it.

The rest of the lifecycle splits into three skills, each usable on its own:

- `watch-pr` waits for the checks. Because it streams one event per check, through the `Monitor` tool under Claude Code, a wait costs one turn rather than one per look. Each poll reads the checks once, a bad response produces an error, a rerun replaces the run it repeats, and a pull request with no checks is inconclusive rather than green.
- `fix-pr` diagnoses a red pull request. One call reads the failing logs and names the local task reproducing each failure.
- `merge-pr` squash merges under a message this toolchain writes. Its briefing script prints the published description, every commit the squash collapses, and the diffstat, then leaves a `SQUASH_AGENTMSG` skeleton whose body the agent writes. That message then passes `mise run lint-squash-msg` and a headless reviewer that `review.sh` starts with `review-squash-message` as its criteria. The stamp it writes binds the draft, the pull request's head, its base, and the commit that base points at. `squash-merge.sh` checks them all again and resolves the confirmation through the `merge` action. It then merges pinned to the reviewed head before it clears both drafts. It also works on a pull request nobody here authored, a dependency bump being the usual case.

Required or changed reviews stop merge preflight with exit 75 and `WAITING_REVIEW`, before message review. The review script checks again before invoking its reviewer, and the merge script checks live state before merging the reviewed head. Merge review builds its complete diff locally from pinned base and head objects, including fork heads. GitHub's capped pull request diff isn't its source.

Left alone, GitHub writes the squash message by concatenating every commit on the branch. That text has never passed a commit-msg hook, and nothing lints it afterwards, so `merge-pr` writes the message instead.

Each of these skills declares guard hooks in its frontmatter scoped to the workflow and inert outside one. The `gh pr` guards work by allowlist: they permit reading through `list`, `view`, `diff`, `status`, and `checks`, and everything that changes a pull request goes through the wrapping script. Naming the read-only verbs rather than the mutating ones means a verb `gh` grows later arrives already covered. `watch-pr` and `fix-pr` refuse the narrower set of forms their own scripts wrap.

A skill's hooks remain active after the turn that invoked it ends, so more than one of these guards is often live at once. The `pr` and `merge-pr` guards share one allowlist so they agree wherever both run. `watch-pr` and `fix-pr` claim only the narrower forms their own scripts wrap, because a broader claim there would refuse a sibling's legitimate calls.

## Repository review policy

Commit, pull request description, and squash message reviews accept optional free-form criteria in `.agents/review-instructions.md`. This path lives outside the deployed skill directories. Repositories without this file keep the built-in review rules. Mechanical leak scans remain separate and keep their existing rules.

### Policy authority

The shared reader in `packages/agents/source/lib/lib.sh` reads the file from the local `origin/<default>` remote tracking ref, never from the working tree, index, or branch under review. Refresh origin before reviewing to get current policy. Default selection comes from `refs/remotes/origin/HEAD`. Without that symbolic ref, the reader uses main, or master when only that local default exists. Applying policy requires a trusted remote tracking commit. Only a regular, non-executable Git blob qualifies as the file.

### Review precedence

The policy overrides conflicting built-in review criteria. A credentials floor follows it in the prompt and always applies: reviewers must still check keys, tokens, `OAuth` client secrets, private keys, and passwords and report exposed credentials. Neither policy text nor an operator note can waive that floor.

Policy supplies review text only. It can't select a reviewer, executable, command, tool, or authority. Reviewer configuration stays on the machine.

A repository managing network policy could adopt this text:

```markdown
Tailnet and RFC 1918 addresses, subnet routes, tags, and member logins in `policy.json` are expected configuration. Allow these network values. Continue to report credentials and secrets.
```

### Policy changes under review

When the reviewed tree changes the policy path relative to the branch's merge base with the trusted default, the entire review uses built-in rules alone. This includes adding or deleting the file and policy changes committed earlier on the branch. An unstaged edit can't supply instructions. Reviews also use built-in rules alone when Git can't establish a common ancestor. Adopt policy changes through ordinary review and the repository's human approval process before using them on later branches. Repository owners may add a CODEOWNERS rule for this path.

### Audit records

Every scripted review reports the policy path, SHA-256 digest of the applied bytes, status, and trusted ref and commit, while the review stamp records the digest, status, ref, and commit. An absent or suppressed policy reports a digest of `none`. Moving the trusted ref during review prevents stamping. Moving it after review requires a new review before committing or publishing.

The same reader and prompt precedence serve commit, PR, and merge reviews on both harnesses.

### Reviewer and Claude Code permissions

`review_cmd` chooses the reviewer. It isn't only a display label. A Codex session needs a reviewer its approval layer permits it to start. Repository machines use `agy` in read-only plan mode through this machine setting in `~/.config/devtools/devtools.conf`:

```text
review_cmd = agy -p "$(cat)" --mode plan --add-dir "$DEVTOOLS_REVIEW_INPUTS" --print-timeout 900s
```

Claude Code operators can put these narrow rules in their personal `permissions.allow` settings after approving the corresponding action:

```text
Bash(bash .claude/skills/commit/scripts/review.sh *)
Bash(bash .claude/skills/agent-commit/scripts/review.sh *)
Bash(bash .claude/skills/merge-pr/scripts/review.sh *)
Bash(bash .claude/skills/agent-merge-pr/scripts/review.sh *)
Bash(bash .claude/skills/pr/scripts/create-pr.sh --rewrite)
Bash(bash .claude/skills/pr/scripts/create-pr.sh --confirmed --rewrite)
Bash(bash .claude/skills/agent-pr/scripts/publish.sh --rewrite)
```

The trailing wildcard covers each review script's optional current arguments and its bare invocation. These scripts have no `--unblock` flag. Permission rules come from an owner, never an agent expanding its own permissions.

## GitHub API quota in pull request skills

PR publication, merge, and watch scripts read the active account through GraphQL `viewer.login`. An exhausted REST core quota doesn't prevent that check. Before a push, publication, or merge, the configured account must still match. A failed request prints GitHub's error. Account mismatches report the actual and configured logins.

For a rate-limit error, the shared reader returns status 76 and prints a message that identifies the limit, followed by the original error unchanged. It stops without retrying the request. GitHub CLI doesn't print response headers, and the message says the failure doesn't reveal the reset time. A `GET /rate_limit` lookup can disagree with the budget the failing call hit. The reader makes none. The guidance depends on the limit. After a secondary limit, wait at least one minute before the next request, as GitHub's docs recommend. No budget query helps. After a primary GraphQL limit (a `GraphQL:` line, a `gh api graphql` call, or a GitHub CLI subcommand other than `gh api`), read the budget once the limit has passed with a GraphQL `rateLimit { remaining resetAt }` query. After a primary REST limit (`gh api` with a REST path), the budget isn't reliably readable from here. Wait for the hourly window and treat `/rate_limit` as advisory only. The reader infers the budget from the command and the error line, and the line says so. Authentication and permission failures keep the original command status rather than becoming an identity mismatch. For ordinary check-read errors, the watcher retains status 2.

### Calls per invocation

This table describes interactive workflows. Unattended merge policy also reads GitHub's applied branch rules through REST when evaluating the pinned base. At the final check, it reuses those pinned requirements and re-reads live approvals and check results. It doesn't poll CI.

These counts describe the explicit readers in the published scripts. GitHub CLI may make extra repository discovery requests or paginate a connection. No reader caches identity across invocations.

| Operation | REST core reads | GraphQL reads |
| --- | --- | --- |
| PR preflight | 0 | Viewer, PR lists, repository, labels, milestones, issues, and published body when comparing a draft |
| PR publication | 0 | Viewer, labels, optional milestones, each distinct referenced issue, final identity check when configured, PR lookup, and current properties for updates |
| Merge preflight | 0 | Viewer reused for the identity report, PR snapshot. PR lookup when the caller supplies no number |
| Squash briefing | 0 | Viewer, one PR snapshot including commits and files, each Related reference. PR lookup when the caller supplies no number |
| Merge review | 0 | Viewer, PR snapshots, base refs, repository URL for fetching pinned objects |
| Merge execution | 0 | Viewer, identity when configured, PR snapshots and base refs for the live gates, final PR URL |
| Watch | 0 | Viewer, optional PR lookup, one checks request per snapshot |

PR writes still use `gh pr create`, `gh pr edit`, and `gh pr ready`. Merge uses `gh pr merge`. GitHub CLI can spend REST quota while resolving reviewers, assignees, labels, and milestones or applying those properties. The scripts don't retry these writes automatically. Git push and fetch use Git transport, outside API quotas. Merge diff review uses pinned local Git objects rather than a REST diff download. The separate fix-pr workflow's Actions run and log readers still use REST through `gh run`. They're outside these read counts.

The rate-limit classifier acts only after GitHub CLI exits non-zero. It then reads stderr for a line that starts with `gh:` or `GraphQL:` and a space. That line must contain `API rate limit exceeded for`, `API rate limit already exceeded for`, or `secondary rate limit`. Matching text on stdout or on a stderr line without those prefixes doesn't count. Successful calls behave as before. A secondary limit need not end at the primary quota reset: follow any retry guidance in the original error. A bare HTTP 403 without a rate-limit message remains a permission failure. The patterns come from the messages of the pinned GitHub CLI release. A reworded message produces an ordinary failure with GitHub CLI's own text instead of a false 76, so a pin bump should recheck these strings.

## Skill sources

APM deploys a package verbatim and has no per-target variant, so the Claude and Codex packages each hold a full copy of every skill. Maintained by hand, those copies drifted in both directions.

A skill with a directory under `packages/agents/source/skills/` has one source instead. Its copies under `packages/agents/claude/.apm/skills/` and `packages/agents/codex/.apm/skills/` are output: edit the source and run `mise run generate-skills`. `mise run check-generated-skills` runs in `mise run lint` and fails on any output file the generator would write differently. A hand edit to the output fails that check before it reaches a consumer.

Sources are siblings of the packages rather than part of one. Without an `apm.yml`, a source has nothing depending on it, and APM never deploys it. Neither do the `packages/agents/*/.apm/skills` globs in `skill-limits.sh`, `check-script-hygiene.sh`, `skill-tokens.sh`, and their `packaged()` helper match it. Those checks keep judging the output that consumers receive. In CI, the `packages/agents/**` path filter in `apm-package.yml` already covers the sources.

Each source directory has a `SKILL.claude.md` and a `SKILL.codex.md`, one manifest per harness. A line reading exactly `@include parts/<file>` pulls in a shared fragment from `parts/`, a directory that stays out of the output. Everything under `shared/` goes into both outputs. Anything under `claude/` or `codex/` goes into that harness's output alone, which is where Claude's guard-hook scripts belong. Without a template for a harness, the generator writes nothing for it. `tokens.json` belongs to `skill-tokens`, so generation leaves it in place and the check ignores it. A package skill with no source directory is hand-maintained and skipped, which lets the remaining skills migrate one family at a time.

`packages/agents/source/lib/lib.sh` is the shared shell library. The generator copies it to `scripts/lib.sh` in every output whose scripts mention it, and a deployed skill then doesn't depend on anything outside its own directory. It contains the harness detection, the machine-level configuration lookup, the script-owned review and its stamp, and the confirmation chain. Its Bats suite and the commit skill’s run in `mise run test-hooks` against the generated output, with stubs replacing the reviewer CLI and the delegation check.

The optional `write-pr-description` helper and the `write-prose-fix` writer run their context scripts as a step rather than during skill loading. Their Claude manifests declare scoped tools for context gathering, edits, and draft validation, so delegated writers can finish without permission prompts. Reviews still run through the script-owned headless reviewer, and confirmation still belongs to the write script’s confirmation chain. Optional live probes use `RUN_HEADLESS_WRITERS=1` or `RUN_HEADLESS_REVIEWS=1`, while ordinary regression runs don't need model access.

The Skill Scanner policy at `.github/skill-scanner-policy.yaml` inherits balanced defaults and suppresses only `ALLOWED_TOOLS_BASH_VIOLATION` for `SKILL.md` in those two writer skills. Scanner 2.3.0 fails to recognize their scoped Bash permissions. The high severity gate still covers other rules, skills, and files, and SARIF keeps suppressed findings with their reasons. Changing that exception requires review.

## Pre-approving the confirmations

`commit`, `pr`, and `merge-pr` resolve confirmation before the step that writes something. Every gate and every independent review has already run by then, so across a series of commits that prompt is the only thing left costing a turn. Answering it once in advance is what these tasks are for:

```bash
mise run preapprove              # commit and pr
mise run preapprove merge        # adds merge to whatever is already held
mise run revoke-preapproval      # withdraws everything
```

Grants are additive. A bare grant covers `commit` and `pr`. The `all` grant covers only `commit`, `pr`, and `merge`. Consumers can declare extra actions with `DEVTOOLS_PREAPPROVAL_SCOPES`. They can check one with `devtools:check-preapproval` and revoke one by name. [Consumer scopes](docs/consumer-preapproval.md) defines validation, exit status, and revocation. Each grant belongs to one session, keyed on `CLAUDE_CODE_SESSION_ID` and then `CODEX_THREAD_ID`. It lives at `<git dir>/preapprovals/<session id>`. A new session starts without a grant. Empty extra scopes work on Bash 3.2 as well as newer Bash.

Granting takes a Touch ID prompt, and revoking takes nothing. An operator's shell and an agent's are the same shell here, reaching the same mise under the same environment, so a fingerprint is the one input left that an agent can't produce. `--no-biometrics` covers a machine with no sensor, and the record says which of the two happened, so a grant that skipped the sensor never passes for one that didn't. None of that amounts to a boundary: anything able to run a shell can write the record by hand, or edit the preflight that reads it. The prompt buys one thing. The sanctioned path needs a person, so an agent granting itself has to bypass that path in a transcript and a diff somebody can read.

Never run either task on the agent's behalf, and never write the record directly.

Every preflight reports what the session holds under `== pre-approval ==`, and the confirmation step reads that line rather than asking again. A grant covers the one question at that step and no more, and each skill lists the cases that send it back to the operator regardless.

In the commit skill, `commit.sh` resolves the confirmation itself rather than leaving it to the skill's prose. That covers runs where no prompt ever appears. It takes the first source that answers, in this order, and prints which one did:

1. `--confirmed`, which the agent passes only after the operator answered yes in this session
2. this session's pre-approval record, keyed on `CLAUDE_CODE_SESSION_ID` and then `CODEX_THREAD_ID`
3. a delegation check configured on the machine as `DEVTOOLS_DELEGATION_CHECK` or `delegation_check` in `~/.config/devtools/<repository>.conf`, called as `<checker> check <repository> commit`, where exit 0 grants and 1 refuses while any other status leaves the commit waiting

Without any of them the script exits 75 and commits nothing, and the agent asks. An unattended subagent may lack a session grant and can't answer an interactive confirmation. The machine’s `delegation_check` supplies that answer. A parent’s instruction alone doesn't configure it. The checker receives `check <repository> <action>`, with `commit`, `pr`, or `merge` as the action. Exit 0 grants, exit 1 refuses, and any other exit leaves the action waiting. A refusal stops the write, and the agent must not grant itself pre-approval or invent an operator confirmation. Like the reviewer, the checker comes from machine configuration only, because a commit must not be able to widen its own authority. The PR and merge scripts use the same confirmation order for their own actions.

For `merge`, the checker always receives `check <repository> merge --pr <number> --head <40-hex sha>`. The head is the reviewed commit passed to `gh pr merge --match-head-commit`. The invocation appends `--owner-path` exactly when the merge gates run and owner paths appear in the merge policy, while checkers should ignore arguments they don't use.

## Moving a branch onto its base

Run the `rebase` skill. It picks the base the way the commit preflight does, then replays the branch under pinned settings and checks what came out.

Conflicts route to `resolve-rebase-conflicts`, which classifies each path before anything edits it. The commit skill rebases at the end of its own run, and a clean replay ends there. Use these two when that rebase stops, when the branch needs moving without a commit in hand, or when `pr`, `fix-pr`, or `merge-pr` reports the branch behind its base.

A rebase that ends without complaint has proved that every commit applied and nothing else. A resolution can drop a hunk. The commit still applies under a message describing work it no longer does, and no gate notices. Comparing the branch against its pre-rebase self through `range-diff` is what catches it. Another failure mode leaves every commit intact and breaks the tree anyway: main adds a gate while the branch adds a file that gate rejects. Neither side was red alone. Since that combination first exists after the rebase, verification runs the gates against the finished tree.

Most conflicts here don't need judgement. `.cspell-words.txt` and the vale vocabulary are append-only sorted lists where the union of both sides is the answer every time. The skill resolves them on that basis and then proves the result is exactly that union. `.gitattributes` marks the generated files:

```text
apm.lock.yaml rebase-resolve=regenerate
**/skills/*/tokens.json rebase-resolve=regenerate
```

A path marked that way resolves by taking one side whole, because the generator overwrites it afterwards. `.gitattributes` also marks every generated skill directory, both packages and both deployed copies, one line per migrated skill. Once the rebase finishes, run `mise run regen-generated`. It measures the generated skill copies before deploying them. Each step reads what the one before it wrote. Commit what the command rewrites.

`conflict-markers=documented` is the companion attribute, for a file whose lines resembling conflict markers are its subject rather than its damage. The resolve skill's own `SKILL.md` has a built-in exemption, so consumers don't see that false alarm.

Never a bare `git stash pop` around any of this. `.claude/rules/worktree-wip.md` explains why, and the guard hook refuses it.

## Prose lint output

`mise run lint-prose` and the prek vale hook already emit the agent output template, which arrives with the `ai-tells` package: `vale sync` writes it to `.vale/config/templates/ai-tells-agent.tmpl`. It prints one self-contained line per finding (location, severity, rule, the exact matched text, and the replacement when the rule defines one) plus a totals line, so you can fix every finding without follow-up searching. Pass `--output=ai-tells-agent.tmpl` yourself only when you invoke vale directly. Empty output means a clean run, and the exit code reports the result. An exit code higher than 1 marks a vale that failed rather than one that found nothing.

That template is a published interface rather than a convenience. The scoping probes in `fix-prose` and `write-prose-fix` count its finding lines with `grep -c '^[0-9]'`, and the replacement probe reads its `replace_with=` field, so reshaping either one is a breaking change for this repository and for every consumer reading the output. Nothing here should reintroduce a repo-local template under a different name. The earlier private copy resolved only in this tree, which left those probes broken wherever that name was absent.

All three of `fix-prose`, `write-prose-fix`, and `review-prose-fix` generate from one source each, with one verdict vocabulary on both harnesses: `PROSE: CLEAN`, `PARTIAL`, or `BLOCKED` from the writer, and `VERDICT: PASS` or `VERDICT: CHANGES REQUIRED` from the reviewer. A run keeps its state in `.fix-prose/run.<key>/` inside the worktree, which ignores itself, so a session isolated in a linked worktree can hand the writer a decision through `<state>/notes`. Old runs may have left `fix-prose.notes` under the git directory, and the context script reads that as a fallback. Running `check-suppressions.sh --allow <state> <path> <line>` records a dictionary word the operator added mid-run. `--verify` then passes on exactly those added lines for `.cspell-words.txt` and a vale `accept.txt`, and it still fails on removed words, extra words, and rule edits.

## Never hard-wrap Markdown

Every Markdown paragraph in this repository occupies one line, and the renderer places its line breaks. Hard wrapping belongs to commit messages and to comments in code or config, each with its own gate.

`cmd/guard-markdown` enforces it. A prek hook runs it over `*.md`. A Claude `PreToolUse` hook runs it against a `Write` or `Edit` and denies the call before wrapped prose reaches disk. `mise run lint-markdown-wrap` runs it in the verification stack, and `mise run fix-markdown-wrap` collapses paragraphs someone already wrapped.

A `CHANGELOG.md` is exempt in every one of those modes. The match is exact, on the base name alone, so a changelog under any directory qualifies and nothing else does. A generator writes it from commit messages, which are hard-wrapped under their own gate. The wrapping arrives with the content, and the next release undoes any unwrapping done by hand. `internal/markdown` holds the exemption in a constant, so it reaches every consumer repo and no repo can decline it. A second exception is the point at which that becomes declarative configuration instead. The vale gate already skips the same file through a glob in `mise run lint-prose`.

Copy this layout for the next rule.

`internal/markdown` holds the rule itself, `internal/hookio` the payload decoding and deny envelope every check shares, and `cmd/guard-markdown` a thin layer of glue over the two. The glue is thin for a concrete reason. `.testcoverage.yml` excludes `main.go` and nothing else, so coverage never measures logic placed there. Putting shared plumbing in its own package also means a second check writes only its own rule. Give each check a standalone binary, and `agenthooks` stays a dispatcher instead of accumulating one subcommand per rule.

APM can deploy a hook's executable. A package-root `hooks/` directory lands under `.claude/hooks/<package>/hooks/`, which a declaration then references through `${CLAUDE_PLUGIN_ROOT}`. This repository doesn't take that route now, so the directory is absent. `guard-markdown` takes the other one: `go install` puts it on PATH. The declaration then specifies a bare command, and no file has to travel beside it.

## Tasks

mise pins the toolchain and holds most task definitions. `mise.toml` has the repo-specific pins and tasks, and it selects the shared payload this repository both publishes and uses itself: the conf.d drop-ins plus the task files under `.devtools/tasks/`, which vendir syncs into every consumer at the same paths. The tree commits both lockfiles, and `devtools:check-toolchain` checks the installed set against them.

That selection runs in both directions. This repository publishes `vendir.toml`, `pins.toml` and `apm.toml` for consumers. Its publisher task bodies remain here. The [consumer adoption steps](README.md#consumer-adoption) document the bootstrap sequence, supported pin forms, task includes, and update procedure. Don't copy the publisher's helper scripts or dictionaries into consumers. The payload supplies draft lint, prose replacements and APM audit helpers, while consumers define prose policy and combined task bodies.

The consumer's `devtools:check-vendored` syncs the payload again and compares it against the index. The Go `devtools:vendor-check` regenerates dependencies and compares the worktree. Both detect untracked output without applying personal ignore files. Neither gate is read-only. Review and stage intentional output before retrying. Shared shell and Markdown exclusions and generated-path extensions live in the [lint guide](docs/shared-lint-exclusions.md). The publisher still runs its Markdown guard from source. Workflow lint skips an empty workflow directory.

[Reusable consumer workflows](docs/consumer-workflows.md) require caller task contracts and stable `lint` and `test` job IDs. They don't replace publisher-specific CI. [Starter policy](README.md#starter-policy) links to consumer starter vocabularies and gitleaks policy. Copy and adapt them outside the vendored tree. Security exceptions require review.

The baseline drop-in pins Go 1.27.3. Scratch Git repositories in the shell suites use `tests/test_helper.bash` to clear inherited Git configuration and identity variables and turn off global and system configuration. Keep new fixtures isolated the same way.

The repository dropped its Justfile. The last recipes to leave were the Go test and coverage family, held back because the CI matrix runs them on Windows and nobody had checked mise there. A probe on windows-2025 found it running a bash-shebang body under Git Bash and resolving its own go pin, which retired both the recipes and the `actions/setup-go` step feeding them. The delegation recipes went with them once the published skills learned to spell their gates `mise run <task>`. The committed `tools/mise-bootstrap` shim still runs in place of mise wherever PATH has none, and the prek hooks run through it.

Tasks that operate on one source language carry a `-go` or `-py` suffix, and the bare name aggregates both: `mise run test` runs `test-go`, `test-py`, `test-hooks`, and `test-leaks` in order. The `cover`, `mutate`, and `fuzz` aggregates each run their Go and Python members. Use the suffixed form while iterating on one language, and the bare form before pushing. Lint gates follow the same pattern through `lint-go-all` and `lint-py-all`, which `mise run lint` composes. A bare-name task lists its members in an ordered `run` array rather than in `depends`, because `depends` runs in parallel and interleaves the linters' findings, which defeats the one-line-per-finding output template.

`mise run test-hooks` runs Bats recursively under `tests/`. Pass paths, such as `mise run test-hooks tests/skills/agent-pr tests/skills/pr`, to narrow it. It runs Bats files in parallel through the pinned `rush`, one job per file, with a job count equal to the CPU count capped at 8. Set `HOOKS_JOBS` to override the count within the cap, and `HOOKS_JOBS=1` for a serial run with an ordered trace. A file that can't run beside the others goes in the `serialFiles` list in `internal/bats/run.go`, with its reason beside it, and runs alone after the parallel group. `mise run test-leaks` runs `.devtools/scripts/check-leaks.bats`. CI runs both in `test-bats (ubuntu-24.04)` and `test-bats (macos-15)`. The Go jobs are `test (ubuntu-24.04)`, `test (ubuntu-24.04-arm)`, `test (macos-15)`, and `test (windows-2025)`. Python is `test (python)`.

Mac measurements on 2026-10-26 used a CPU count of 16 and the same 744 cases. Wall times include discovery and the helper build. The serial run sets `HOOKS_JOBS=1`. The parallel run with 4 jobs sets `HOOKS_JOBS=4`. The default run uses the CPU count capped at 8. Existing skips cover authenticated writer probes, Rust checks without the required toolchain, and optional Renovate and Skill Scanner probes.

| Run | Jobs | Wall time (seconds) | Passed | Failed | Skipped |
| --- | ---: | ---: | ---: | ---: | ---: |
| Serial | 1 | 1113.52 | 735 | 0 | 9 |
| Parallel | 4 | 353.35 | 735 | 0 | 9 |
| Default | 8 | 267.50 | 735 | 0 | 9 |

`fix-pr` maps check names by matching text within them, in order, rather than by workflow job ID. `lint` routes to `mise run lint`. All those `test` names, including `test-bats`, route to `mise run test`. For a Bats failure, the focused CI reproducers are `mise run test-hooks` and `mise run test-leaks`. A name containing `race` routes to `test-go-race`, `cover` to `cover-go-check`, `vendor` to `vendor-check`, `vuln` to `vuln`, and `gitleaks` or `secret` to `devtools:gitleaks`. `validate` routes to `apm install --frozen && apm audit --ci`. An unknown name, including `scan-skills` and `gomodscan`, requires reading the log because the diagnosis script doesn't invent a reproducer. The Actions run and log reads still use REST.

`mise run lint-shell` delegates to `devtools:lint-shell`, which enables shellcheck's `check-extra-masked-returns` rule (SC2312). `devtools:lint-workflows` enables it for workflow shell blocks too. It flags command substitutions and other command uses that mask a failure status, such as testing a substitution's output directly. Capture the output separately and check the command's status first. SC2155 remains enabled for declarations that mask command failures.

For a harmless SC2312 finding, put `# shellcheck disable=SC2312 # diagnostic formatting cannot change the failed verdict` immediately before the affected command, replacing the reason with the case's actual justification. Use a local exception only when failure can't affect correctness.

`mise run devtools:lint-curl` checks owned shell scripts for curl calls without `--fail`, `-f` (including short-option bundles), or `--fail-with-body`. This includes bodies piped to another command or captured in a substitution. Bodies written to a file also qualify. It accepts `-w '%{http_code}'` or `--write-out '%{http_code}'` when it recognizes a comparison of the result. For an intentional error-body check, put `# lint-curl: allow the test asserts on the HTTP error body` immediately before the curl command's starting line, replacing the reason with the actual justification. Supply a reason. The comment exempts only the first curl call on that next line.

Never accept pre-existing findings through a baseline. Fix them or justify a harmless case with a local comment. Don't turn off a rule or exclude owned source paths to make existing findings pass. Commands hidden behind `eval`, variables, wrappers, or quoted remote shell strings still need review.

Python tooling covers `packages/`. It came from `proofhouse/proofhouse-python-tool`, dropping the checks that need an installable package. Because `pyproject.toml` declares a virtual project (`package = false`), `import-linter` contracts, a wheel build, and a generated build stamp have nothing to act on here. `lint-reuse` is absent for a different reason. REUSE compliance spans every file type in the tree. Adopting it needs a whole-tree sweep plus a `REUSE.toml` and `LICENSES/`, so that work belongs in its own change.

The `[tool.pyrefly.errors]` table pins every diagnostic kind pyrefly knows. That catalog is version-specific, and an unknown name fails the entire config load, so regenerate the table on every pyrefly bump. Naming an unknown kind makes pyrefly print the full list of accepted values.

## Releases

The release happens in CI and nowhere else. `.github/workflows/release.yml` runs on dispatch. It generates an app token and runs `cog bump`, replays that commit through `createCommitOnBranch` so GitHub writes and signs the one that main receives, then SSH-signs the tag with the organization secret `TAG_SIGNING_KEY` under the tagger identity `tbhb.dev Agent <334376894+tbhb-agent@users.noreply.github.com>` that GitHub verifies the key against.

Two signers, because neither one can do the other's job. Signing a commit on the runner takes a key the runner has, and a GitHub App has no such key, so only GitHub can produce a commit under the App's identity. It signs with its own key, and the result names `tbhb-dev-releases[bot]` as author and GitHub as committer. The tag runs the other way. No API creates a signed tag, so the runner's key is the only thing that can sign one. cog's commit never leaves the runner. The workflow reads the message and the changed paths off it, and the API writes the commit main ends up with. A tree comparison checks those two against each other before the tag goes on. Every release CI cut, v0.3.0 through v0.5.0, signed the tag alone, so none of those version bumps has a signature. A person cut the releases before them by hand, and signed those.

No local command reproduces that, so this repository has no bump-version task and no local tagging path. The version comes from Cocogitto and nowhere else, derived inside CI, so a second bumper on a development box would be a second source for one number.

Run the `release` skill. It composes the three tasks below and decides their order, and this paragraph and the next only summarize them. Each task remains useful alone. Readiness can run on any Tuesday to check whether the repository is releasable, and verification can run against a tag cut months ago.

Those three exit non-zero on failure. None prompts for anything, and none fixes anything. Readiness reports rather than cleaning the tree or moving a literal, because nobody can run a primitive that mutates on the way to an answer just to ask a question.

- `mise run check-devtools-release-readiness` runs before a dispatch. It checks that the working tree is clean, `HEAD` is the commit `origin/main` points at, the checks on `HEAD` are green, the version literals match the latest tag, and the changelog previews. It tests no branch name, deliberately. Only one checkout in a repository can hold main. An earlier check asserting the name failed by construction in every agent worktree, which is where the work happens, and the v0.5.0 dispatch went out around it. The tag doesn't depend on anything local anyway: `gh workflow run` dispatches on GitHub and the workflow checks main out fresh, which leaves every check here asking whether the operator's picture of the release is accurate, and the commit comparison checks that under any branch name and detached alike. `check-all` is deliberately absent from that list, because it reaches `tidy`, and a preflight composing `go mod tidy` would rewrite `go.mod` on the way to an answer. The checks-green probe covers the same checks by asking GitHub what they concluded about this exact commit, which is the better answer anyway: it came from a clean runner on every platform in the matrix. Run `check-all` by hand where a local sweep is what you want. Readiness also prints the version `cog bump --auto` would derive. That number comes from the Conventional Commit types since the last tag, so a run of fixes and chores yields a patch where the operator wanted a minor, and #25 exists because nobody saw that difference until after the tag.
- `mise run release-devtools [X.Y.Z]` dispatches the workflow and passes the version through, or omits it for the automatic path. It refuses where readiness fails. It's deliberately thin. It names `--ref main` rather than letting the bare form resolve to the default branch, because `on: workflow_dispatch` takes no branch filter and constrains nothing, while `release.yml` checks out whatever ref carried the dispatch and pushes to main outright. The target for release management here is a continuously updated release pull request whose merge tags and publishes. Under that model releasing is merging, so that model replaces this piece, while the other two remain in use.
- `mise run verify-devtools-release [vX.Y.Z]` runs after the workflow run finishes. Its sharp assertion is that the tag has an annotation and a signature rather than merely existing, because cog drives libgit2 and can only make a lightweight tag, which `push --follow-tags` leaves behind silently. The workflow deletes and re-creates the tag signed to compensate, so a check asking only whether the tag exists would pass in exactly the case that workaround exists for. It asks the same of the commit under that tag, separately, because the two carry different signatures from different signers and a good tag says nothing about the object beneath it. Reading the tag alone is how every CI-cut release from v0.3.0 on passed verification over an unsigned commit. No tag published so far clears both assertions: v0.3.0 through v0.5.0 fail the signature, and the hand-made v0.1.0 through v0.2.0 fail the authorship, since a person signed those rather than the release app. The next release is the first that can pass. The bump also runs under `--skip-ci`, which leaves this task as the only thing reading the release commit before consumers pin it.

The tag is the whole release. Consumers resolve the ref directly through apm, the Go module proxy, a `.pre-commit-config.yaml` rev, or a workflow pin, so this repository doesn't publish a GitHub Release object, release assets, or built artifacts. Verification looks for none of those, and finding none is the correct answer rather than a gap.

The skill runs every one of them against a throwaway clone of the release branch rather than against the checkout you invoke it from, which is what lets it work from any worktree without touching the local main checkout. `release-clone.sh` makes that clone and a guard hook refuses a release task run any other way. Convenience isn't the reasoning. The release is a statement about `origin/main`, since the workflow builds that branch fresh on the runner, so a verdict read out of whatever local tree happens to be in front of you describes something nobody is releasing. `--reference` against the local object store keeps the clone under a second, and `install-toolchain` inside it means the release branch's own pins decide which cog judges it. The clone makes the clean-tree and current-with-origin checks true by construction, so what those checks used to catch arrives instead as a `NOTE` from `prepare` naming the local commits the release leaves behind. Advisory rather than gating, because a branch in flight is the normal state here.

The skill adds the judgement the tasks can't hold. It confirms the version before dispatching anything, which matters because the automatic path derives one from the commit types and #25 exists because that differed from what the operator wanted. `review-release-version` is the second opinion on that number, a forked reviewer reading the commits and what they publish to consumers rather than the types, because cog can't see that a `build:` commit rewriting the vendored payload affects every consumer. It waits on the dispatched run through its own watcher, since `gh workflow run` returns before the workflow has tagged anything, and that watcher reports what it watched and says nothing about the branch: the workflow pushes the release commit to main before the tag, so a failed run routinely leaves main moved. Guard hooks refuse local tagging, local bumping, a dispatch that goes around the task, and a release task run outside the clone.

The root `apm.yml` is a workspace and consumer manifest now, depending by local path on four sub-packages under `packages/agents/` rather than declaring any primitive itself. This repo's own `apm install` deploys them here in the same pass, which is how it consumes what it publishes. `common` has no `targets` restriction, so it deploys to every active target. `claude` contains the skills plus the `guard-markdown` hook declaration, and its own `targets:` field limits it to the `claude` harness alone. `codex` contains the Codex versions of the same skills under the same names, limited to the `codex` target. Claude Code reads only `.claude/skills/` and Codex reads `.agents/skills/`. Those copies never share a directory, and identical names don't collide. `agy` is a scaffold, limited to its own target and empty until it gets a first primitive. All three targets are active here, so `common`'s one primitive, the `worktree-wip` instructions, deploys twice: `.claude/rules/worktree-wip.md` and `.agents/rules/worktree-wip.md`. That second mirror is inert for the `agy` CLI today. It has `trigger: glob` frontmatter. Only the Antigravity IDE honors that trigger kind. The CLI itself loads just `trigger: always_on` rules plus the root `AGENTS.md`, an upstream defect that deploy-side workarounds can't fix. Codex doesn't get an install-time instruction deploy at all, since reaching it needs `apm compile` into `AGENTS.md` and this repo doesn't run that: the file is hand-authored, and `compile` currently has an upstream bug that bypasses package target restrictions. A consumer pins a sub-package directly, as `tbhb-dev/devtools/packages/agents/claude#vX.Y.Z`.

`release` and `review-release-version` live under `.claude/skills/` with no counterpart in any `packages/agents/*/.apm/`, which makes them the first repo-local primitives here. They release this repository, so no consumer should receive them. That placement passes `apm install --frozen`, `apm audit --ci`, and the working-tree drift check in `apm-package.yml` untouched. The installer leaves a file it never deployed alone. The drift check reads `git status`, and a committed file leaves that clean. The skill-scanner job reads `packages/agents` recursively and never sees it, since scanning deeper finds nothing new there. `skill-limits.sh` and `check-script-hygiene.sh` do, because each derives the local-only set by asking whether any `packages/agents/*/.apm/skills/<name>` exists, the shared `packaged()` helper. Those two checks cover the next one when it arrives.

`mise run check-versions` is the fourth piece and the only one `mise run lint` composes. `cog.toml`'s `pre_bump_hooks` rewrite eleven published version literals during a bump, and that task is the witness that they did. The compatibility script forwards to the repository-local Go command through `mise run run agentrepo check-versions`. See [version checks](docs/version-checks.md) for discovery, diagnostics, and explicit versions. Its name omits the prefix because it checks a tree rather than releasing a product, which is what the `devtools` qualifier on the other three marks.

## Verifying Claude Code behavior

The public Claude Code docs don't always match the installed version. When the behavior of a hook or harness feature matters (which events fire, in what order, whether an event can block, what fields its stdin payload includes), confirm it against the installed `claude` binary rather than trusting the docs or prior memory.

Probe it with a throwaway project instead of reasoning about it:

1. Create a scratch project under a temporary path with its own `.claude/settings.json`.
2. Register a small logging hook on the events in question. Have it read stdin and append `hook_event_name` plus the fields you care about to a log file.
3. Drive it headless: `claude -p "<prompt that triggers the tools>" --permission-mode bypassPermissions --model claude-haiku-5-5 < /dev/null`.
4. Read the log to see what fired.

The preceding probe settled a question for this repo's hook: `PostToolUse` fires once per tool call and includes `tool_input.file_path`, while `PostToolBatch` fires once per batch (one call still counts as a batch) and has no per-tool fields.

Against Claude Code 2.1.220, a later probe settled the questions the commit skill's guard hooks rest on. Frontmatter hooks in a `SKILL.md` file do fire, and `${CLAUDE_PROJECT_DIR}` expands inside the `command`. Exiting 2 from a `PreToolUse` hook blocks the call from that scope, same as from one configured in settings. For the Skill tool, the `PostToolUse` payload names the invoked skill at `tool_input.skill`, and has no `tool_input.skill_name` of the kind the published reference describes.
