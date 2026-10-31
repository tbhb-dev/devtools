# Migrating unattended skills

The package includes generated `agent-commit`, `agent-pr`, and `agent-merge-pr` skills for Claude and Codex. The package version comes from the release workflow. Commit owner-path delegation and external reporting remain under [issue 241](https://github.com/tbhb-dev/devtools/issues/241).

## Consumer cleanup

Move the supported settings and machine authorities before replacing local skills. The generated merge skill requires `uv` for its pinned YAML reader and `agentrepo checks evaluate` for check judgement.

### Skill limits fallback retirement

After the N-2 window, the follow-on Feature [issue 847](https://github.com/tbhb-dev/devtools/issues/847) removes `tools/skill-limits-legacy.sh` and the capability probe in `tools/skill-limits.sh`. Consumers get at least two releases of warning, counted from the first release containing this notice, and the note doesn't set a removal version.

The shared loader requires `agentrepo` v0.16.0 or newer for release binaries. A source build, which reports `DEV` or a `v0.0.0` pseudo-version, skips the floor. It resolves `mise which agentrepo` from the repository root first, then uses `agentrepo` from `PATH` if mise supplies no executable. Configuration reads and review execution use that binary, as does check judgement. An older binary stops the loader with the found and required versions. When neither source provides `agentrepo`, the shell keeps parsing configuration as before.

For avirt and the other existing consumers, pin the harness package containing the replacement skill, then install it. Remove each replaced local skill's APM dependency and `apm.yml`. Delete its local scripts and Bats suite under `.agents/skills/agent-commit/`, `.agents/skills/agent-pr/`, `.agents/skills/agent-merge-pr/`, and their `.claude/skills/` counterparts as applicable, allowing APM to deploy the generated copies. In avirt, remove the replaced commit, publication, and merge suites from `test-skills`. Remove that task and its call from `check` only when no consumer tests remain. Other consumers remove the same replaced suites from their test tasks. devtools runs the shared regression coverage through `mise run test-full`. Consumers keep tests for their own hooks and repository settings.

Keep a local commit skill if its delegation checker requires protected-path arguments or its workflow requires external reports: the commit delivery doesn't support those settings yet. Publication now supports both owner-path delegation and reporting.

Keep the old configuration while any local skill still reads it. Move supported behavior to `.agents/agent-skills.conf` and authorities to machine configuration before deleting it.

After replacing the local merge skill, remove its compatibility copy of `legacy-agent-skills/scripts/lib.sh` and `legacy-agent-skills/agent-skills.conf` outside the deployed directories. Keep that copy only while a local commit awaiting replacement still needs it. A consumer retaining its local commit for owner-path delegation or reporting must keep that skill's configuration and tests until those features migrate.

Install the sibling `commit`, `pr`, `merge-pr`, and review skills from the same package. The unattended entry points delegate to them. These skills supply the shared gates and publication machinery.

To adopt base merge mode, pin a package containing it and run `apm install`. Use `gh pr update-branch` when the base merges cleanly. For conflicts, fetch origin, merge `origin/<base>`, resolve conflicts, stage paths by name, and finish through `agent-commit`. It accepts only the current fetched PR base tip, or the default branch tip without a PR. Keep the repository's merge-subject rule and required trailers. Skip the post-commit rebase after a base merge.

Their interfaces remain available for the later `agentrepo` port.

## Cached gate passes

Pin the harness package containing this change and run `apm install`. Saved passes remain reusable while their cache key is unchanged. This also applies to message and description lint passes. The key still requires a clean committed tree and matching commands, tool versions, and gate inputs. Message and description lint also bind their drafts. Use `--rerun-gates` to force fresh runs.

## Publication leak scans

Pin the harness package containing this change, run `apm install`, and sync the `.devtools` payload so `check-leaks.sh` and `leaks.toml` match. `pr` and `agent-pr` then scan only what the branch adds after its merge base with `refs/remotes/origin/<base>`, so list values already in the base no longer stop a publication. A missing, shallow or unrelated base exits 2 before any push, and `mise run lint` under publication is no longer a cached gate pass. The consumer's pre-push wrapper is the same as before. CI's `--range` scan and a bare `devtools:lint-leaks` still scan everything.

## Generated skill conflicts

Pin the harness package containing the updated rebase skills and run `apm install`. In the devtools checkout, run `mise run regen-generated` after resolving generated files during a rebase, then commit what it rewrites.

`agentrepo markers scan` now scans rebase conflict markers in Go and accepts an empty path as a support probe (`agentrepo markers scan ''`), without reading any files. The shell fallback keeps older binaries working. Consumers use their existing configuration. Remove the fallback in a later release after [issue 333](https://github.com/tbhb-dev/devtools/issues/333) and [issue 544](https://github.com/tbhb-dev/devtools/issues/544) land and the N-2 compatibility window passes.

## Repository configuration

The file accepts `key = value` lines, blank lines, and full-line comments. Its reader never sources it as shell. Unknown keys, duplicates, and symlinks fail. Commands execute only at their configured gate. Other values remain data. Review stamps bind the configuration bytes. Commit the repository configuration before reviewing a branch.

```ini
lint_commit_cmd = mise run lint-commit-msg
required_hooks = commit-msg pre-commit post-commit
hook_marker = File generated by prek
review_max_rounds = 3
pr_template = .github/pull_request_template.md
lint_pr_cmd = mise run lint-pr-description
pr_log_entry = optional
pr_related_pattern =
pr_title_check_cmd =
owner_paths = ^$
mail_cmd =
mail_to =
mail_from =
lint_squash_cmd = mise run lint-squash-msg
owner_review_label = owner-review
ruleset_dir = .github/rulesets
golden_paths = ^$
constitutional_paths = ^$
loosening_markers = nolint|shellcheck disable|vale (off|disable)
gate_config_paths = ^(lint|hooks)\.(yaml|toml)$
disposable_ignored = ^cache/
merge_ready_cmd =
```

The commit and publication values preceding are defaults. Merge requires explicit nonempty `golden_paths`, `loosening_markers`, `gate_config_paths`, `owner_paths`, and `disposable_ignored` patterns, while an unset `constitutional_paths` defaults to `^$` and a present empty key fails. Use `^$` for a deliberately empty set. The merge examples are placeholders for consumer policy, not built-in protected paths. The round bound accepts integers from 1 to 9. Success resets the corresponding finding counter. Reaching the bound blocks further reviews of that subject even after preflight.

devtools defines its own patterns in `.agents/agent-skills.conf`. They protect the published payload and hooks, workflows, rule sets, gate configuration, and skill guards. Cleanup permits known build and cache output and preserves local settings, notes, dependency directories, and other worktrees. Consumers must choose patterns for their own trees. Policy refusals print the exception message. An empty pattern reports its key.

When the unattended finding bound returns exit 3, run `agentrepo review redesign KIND --reason TEXT` with `KIND` set to `commit`, `pr` or `squash`, then rewrite the draft from scratch. A rejected digest for the same subject refuses review before invoking the reviewer. Each active record permits one redesign. A bound after redesign requires inspection and refuses another start. Legacy records without rejected digests require inspection. The command leaves write authority and every other gate unchanged. A fresh description reset, where supported, remains separate from redesign.

The per-kind files `agent-skills/commit.rounds`, `agent-skills/pr.rounds` and `agent-skills/squash.rounds` live under the worktree's Git directory. New records begin with `subject TEXT` and `bound N`, followed by `finding SHA256` lines. A redesign appends a `redesign JSON` line with `kind`, `subject`, `at` (UTC), `reason`, `bound` and `rejected` (the rejected draft digests). JSON escapes newlines in reasons so each event stays on its own line. Finding lines after the last redesign event determine the current epoch. Earlier findings remain visible in review output. A new subject uses the existing fresh-record rule and inherits no redesign. Successful commit, publication, or merge removes the corresponding record. Failed reviewer execution and changed inputs don't count as findings.

Consumers must update the skills and the `agentrepo` binary together before using redesign. Legacy count records remain held because they can't identify which drafts the reviewer rejected. Don't edit or remove records to restart a review.

`pr_template` names a file inside the repository. Scaffolding, shared validation, and the committed review snapshot use it. An empty `lint_pr_cmd` fails. The existing `PR_TEMPLATE` environment variable remains the fallback when the repository doesn't specify a template.

Set `pr_log_entry = required` to require the final line containing text to have the form `Log entry: * **Label**: text`. The default leaves that line optional. A nonempty `pr_related_pattern` is a grep extended regular expression applied to each comma-separated Related item after removing a bullet. Anchor it to match whole items. Use `None` or `None.` only alone. The pattern `^#[0-9]+$` permits issue references. An empty pattern leaves shared template validation in charge.

The optional `pr_title_check_cmd` receives `AS_BASE`, `AS_HEAD`, and `AS_TITLE`, preserving the public consumer's command interface. The base resolves to origin's copy when available, `HEAD` is the committed branch, and the title is exact. Publication stops if the command returns a nonzero status.

`owner_paths` is an extended regular expression, preserving the previous setting's syntax. It matches each changed path from the branch's merge base. The pattern `^(docs|policy)/` covers either directory. Deletions and both sides of renames count. Matching reads paths delimited by the null character (`NUL`), so a newline in a filename doesn't split it. A match adds `--owner-path` to the machine checker invocation: `<checker> check <repository> pr --owner-path`. Without a match, its existing arguments stay the same. Authority doesn't come from this signal. Session pre-approval still covers the `pr` action.

Reports print locally unless both `mail_cmd` and `mail_to` are nonempty. The command must be an executable path. Shell expressions aren't accepted. Publication invokes it as `send --to <recipient> --kind fixed --body <report> --pr <number>`, adding `--from <sender>` when configured. A delivery failure prints `REPORT NOT SENT` with the report and leaves publication successful. It never repeats the GitHub write. Merge success uses the same transport with kind `fixed`. Merge holds and uncertain outcomes use kind `blocker` and preserve the original exit status.

## Settings mapping

| Previous setting | Destination |
| --- | --- |
| `lint_commit_cmd`, `required_hooks`, `hook_marker`, `review_max_rounds` | Same keys in `.agents/agent-skills.conf` |
| Template selection | `pr_template` in repository config |
| `lint_pr_cmd`, `pr_log_entry`, `pr_related_pattern`, `pr_title_check_cmd` | Same keys in repository config |
| `owner_paths` | Same key for publication and merge delegation and reporting. Commit migration remains pending |
| `mail_cmd`, `mail_to`, `mail_from` | Same keys for publication and merge reports. Commit reporting remains pending |
| `signoff_identity` | Optional machine configuration under `~/.config/devtools/`, required when the Git identity is an agent |
| `review_cmd`, `review_model`, `review_effort`, `delegation_check`, `gh_login`, `required_reviewer`, `required_reviewer_type`, `merge_owner` | Machine configuration under `~/.config/devtools/` or corresponding `DEVTOOLS_` variables |
| `review_commit_criteria`, `review_pr_criteria`, `review_squash_criteria` | Installed review skills plus the existing trusted repository review policy |
| `lint_squash_cmd`, `owner_review_label`, `ruleset_dir`, `merge_ready_cmd` | Repository configuration. Readiness can refuse but can't grant authority |
| `golden_paths`, `constitutional_paths`, `loosening_markers`, `gate_config_paths`, `disposable_ignored` | Repository configuration. Inspect patterns before migration |
| `merge_verify_tries` | Retired: shared merge execution reads the result once and never retries the write |

Repository configuration can't select a reviewer, GitHub account, or delegation checker. Unattended publication requires a nonempty machine `gh_login` with no built-in account default. It rejects `--confirmed`. Either session pre-approval or machine delegation must grant the action, and neither skill prompts or grants itself authority.

The machine's `delegation_check` receives `check <repository> <action>`. Exit 0 grants, exit 1 refuses, and any other exit leaves the action waiting. For `merge`, `--pr <number>` and `--head <40-hex sha>` are always present: `<checker> check <repository> merge --pr <number> --head <40-hex sha>`. Its head is the reviewed commit passed to `gh pr merge --match-head-commit`. The invocation appends `--owner-path` exactly when the merge gates run and owner paths appear in the merge policy, while checkers should ignore arguments they don't use.

## Behavior comparison

The public source is the [avirt local skill family](https://github.com/tbhb-dev/avirt-poc/tree/main/.agents/skills). Comparison with the original consumer found a missing publication delegation check in that public copy. The shared confirmation chain supplies it. No consumer identities, mailbox values, or protected-path patterns belong in the package.

| Behavior | Shared behavior |
| --- | --- |
| Snapshot review, strict verdict parsing, input drift detection | Existing review and stamp machinery |
| Machine delegation and session pre-approval | Existing confirmation chain without prompting |
| SSH commit signing and hook markers | Shared commit signing helpers |
| Failed commit verification | Preserve the commit and evidence for recovery instead of automatic rollback |
| Description template, lint, log, Related grammar, and title checks | Shared validator and configured checks before publication |
| Labels, reviewers, assignees, milestone, and draft state | Existing publisher converges on the reviewed properties |
| Identity | Require the machine account before push and again after push |
| Push leases and post-push verification | Existing publisher, with unattended branch, draft, configuration, and clean-tree checks |
| Review finding bound | Persistent shared counter that remains for unpublished drafts and resets on success |
| Reporting | Local output and a structured result, with optional configured transport |

Publishers must omit `owner_review_label` from draft frontmatter. The setting defaults to `owner-review`, as it does for merges. Preflight excludes it from label choices. Only the owner-approval route applies the label after approval at the current head, green checks, and a mergeable state. Updates preserve an existing owner-review label while reconciling other labels normally. Regenerate and reinstall the skills to use this rule.

A successful publication records `pr.result.json` under the worktree's Git `agent-skills` directory, containing the PR number, URL, full head, base, draft state, and push kind. Final output reports the URL, head, and draft state. Inspect a failed or uncertain GitHub write before retrying. After publication, the skill stops.

## Merge policy and authority

Piece A of the merge split adds the optional `agentrepo config merge-machine KEY [DEFAULT]` reader. It rejects any set `DEVTOOLS_` variable for `merge_owner`, `required_reviewer`, `required_reviewer_type`, `delegation_check`, or `gh_login`. This includes empty variables and variables unrelated to the requested key. Values come from the machine's repository file, shared defaults, then the caller's fallback. Empty file values still win. No workflow calls the reader until piece B. Existing keys, environment overrides, defaults, and gates keep their behavior through the N-2 compatibility window.

Machine configuration must name `gh_login` and `required_reviewer`. The latest decisive review from that reviewer must approve the exact current PR head. A comment or pending review supplies no approval, and a later change request or dismissal blocks. Use the GraphQL login without the REST `[bot]` suffix. Optional machine `required_reviewer_type` accepts `Bot` or `User`. Leaving it unset keeps login-only matching. A wrong author type can't approve the head. A GitHub App installation needs Contents read and write permission for its approval to count. [#300](https://github.com/tbhb-dev/devtools/issues/300) documents the installation permission and down-scoped review tokens. The shared reviewer CLI remains selected only through machine review settings.

Golden-file changes and detected gate loosening require an APPROVED review from optional machine `merge_owner` on the same exact head. Configure it through `dt_conf`, such as `merge_owner` in machine configuration or `DEVTOOLS_MERGE_OWNER`. Use its GraphQL login without `[bot]` too. Keep `merge_owner` a person, since an App owner override can approve its own golden-file waivers. The reviewer type setting applies only to `required_reviewer`. Repository configuration can't name that login. A push invalidates approval.

Golden findings alone can proceed without owner approval when the required App's latest decisive review approves the exact current head and supplies the header-position statement below. The format is `Loosens: yes|no`. Only `no` permits the waiver.

Normalize carriage return plus line feed (`CRLF`) to line feed (`LF`) before checking the body. Every character in the whole body must belong to printable `ASCII` (`0x20` through `0x7E`), or be a tab or `LF`. Unicode whitespace, form feed, `NUL`, and an unpaired carriage return require the owner.

The body must begin with `Reviewed head: <40 lowercase hex digits>`. Review authors may put the SHA inside one pair of backticks. The header SHA must equal the merge review's pinned head. A mismatch requires the owner. An optional `Model: <letters, digits, dot, dash, underscore, space>` line may follow. Empty lines may separate these header lines and the statement. The first line after this header block must be exactly `Loosens: no`. Surrounding whitespace or extra text requires the owner. The following line must be empty, or the body must end. Ignore later text except for the whole-body character check and duplicate statement check. The App review brief must place `Loosens: no` in this header position.

Missing, duplicated, or ambiguous statements fail closed and require the owner. Any value other than `no` or a statement outside this header position requires the owner. Only the permitted header lines can precede the statement, so quoted, list-nested, code-span, comment, fenced, or indented text can't supply it. A review by another login, a non-App author, a stale head, or an edited review can't supply the waiver. Missing edit metadata also requires the owner. The policy reads the live review again before merging, so editing an old review after a push can't revive its authority. Reviewers receive their code-review instructions outside this repository.

The merge policy reads `constitutional_paths` from `.agents/agent-skills.conf` at the pinned base commit rather than the pull request checkout. An unset `constitutional_paths` at the base defaults to `^$`, while a present empty key fails. Paths matching `constitutional_paths` require the owner, even with `Loosens: no` or remote APM ownership, subject to the pin exception below. devtools protects its merge policy configuration, repository review instructions, rule sets, CODEOWNERS, Claude settings, and merge authority sources and generated copies: `merge-policy.py`, `merge-agent.sh`, `signing.sh`, and `lib.sh`. Detected gate loosening also always requires the owner. Listing `.agents/agent-skills.conf` in `constitutional_paths` prevents a policy edit from waiving its own owner check because the merge policy reads the key from the base commit rather than the pull request checkout.

Pin the harness package containing the updated pin reader and run `apm install` before relying on the exception. A base with an older reader retains its existing holds. Existing devtools version pins in `apm.yml`, `mise.toml`, workflow files, `.github/renovate.json5`, `vendir.yml`, `.pre-commit-config.yaml`, and the adapted header comment in `tombi.toml` can change without constitutional or golden findings when every changed protected file contains only recognized in-place pin replacements. Renovate refs may move from a version tag or SHA to a new SHA with the version comment maintained. Every replacement preserves its surrounding text, including mise comments. Contextual pins require the immediately preceding repository key at the same mapping indentation, allowing a sequence marker. The reader rejects nested keys, non-comment URL suffixes, and embedded line separators to keep unrelated configuration edits outside the exception. A non-pin edit in the same protected file or another protected file leaves the existing findings unchanged. Added pins, unrecognized forms, and pins for other repositories keep their findings. The protected path patterns, owner path report, suppression scan, and approval chain remain in place, including suppression findings on recognized replacements carrying an unchanged suppression comment.

Reports include the rule used: `owner approval` or `App review states Loosens: no`. They also record whether execution used an override and the approving login. The App approval, required checks, independent review, and write authority still apply. The owner-review label still holds a PR for the owner, and neither rule waives it or any other merge gate.

The scan skips full comment lines. It recognizes exclusion keys instead of matching words inside values and ignores suppression text inside quoted values. Non-local APM deployments come from `apm.lock.yaml` at the pinned base commit. Local deployment overrides stay in scope. A missing lockfile means no deployment exemptions. Malformed ownership data refuses the scan. A PR can't grant itself an exemption by editing its lockfile. The scan makes a narrow exception for each of these, without configuration. A `mise.lock` diff that adds at least as many `checksum` lines as it removes doesn't count the removed checksums as removed gates. An inline `//nolint` in a `vendor/**/*.go` file doesn't count as a suppression when `vendor/modules.txt` exists at the base and changes in the same diff, and any other marker on that line still does. Consumers adopt both through a normal pin bump. The scan is a backstop and doesn't replace review.

Required checks come from GitHub's applied branch rules and active ruleset JSON files in `ruleset_dir` at the pinned base. Requirements match names exactly. The merge requires a nonempty list of known required checks and a nonempty rollup. Skipped required checks, unknown states, and missing results refuse the merge. Check judgement uses `agentrepo checks evaluate` instead of another check judge. A configured `merge_ready_cmd` must be an executable path and receives `<repo-root> <pr> <head>`. Its nonzero status blocks, while empty readiness doesn't add a check.

`merge-pr` also enforces the full merge gates when confirmation comes from session pre-approval or machine delegation. Only an explicit `--confirmed` representing an operator answer keeps the attended behavior. Every execution refuses an empty check rollup as inconclusive.

The workflow environment variable is `DEVTOOLS_UNATTENDED`. `DT_UNATTENDED` remains a deprecated alias for one release: the reader uses `DEVTOOLS_UNATTENDED` when set, otherwise `DT_UNATTENDED`. Generated entry points and child review processes pass the new name. The context isn't exported while repository gates and reviewers run.

Existing lint, review, stamps, and the `merge` confirmation chain serve these scripts. Unattended execution rejects `--confirmed`. Use session pre-approval or machine delegation. Owner paths add `--owner-path` to merge delegation and appear in the report. `merge.sh <pr> <expected-head>` preserves the previous explicit head interface. Shared stamps also pin that head. Failed reviewer execution, timeout, and input drift don't consume a finding round.

Before merging, execution changes the base of dependent pull requests only for a same-repository head. Execution never retries a GitHub write automatically. It verifies the resulting merge commit and its message before cleanup. An unknown outcome or message mismatch preserves evidence. Remote deletion leases on the reviewed head, fork branches stay, and local removal preserves the current worktree, non-agent worktrees, changed branches, dirty trees, and ignored files outside `disposable_ignored`. Reports keep the full merge commit and any owner override, even if later cleanup is incomplete.

The remaining issue 241 work is commit-specific owner-path delegation and external commit reporting. Consumers requiring those behaviors keep their local commit copy until its replacement supports them.

## Pull request failure diagnosis

`agentrepo checks diagnose [pull-request-number]` reports PR identity, checkout context, failing checks, suggested local tasks, and bounded failing-step logs. Without a number it selects the current branch's open PR. It reads each workflow run's log once and never retries requests or executes suggested tasks. `FIX_PR_LOG_LINES` sets the log tail length. The default is 80.

Exit status 0 means a readable, nonempty check set has no selected failure state. It doesn't mean every check passed. Known failures return 1. Missing, malformed, empty, or unreadable PR/check data returns 2. Unavailable logs for known failures keep the cancellation notice and status 1.

Pin the harness package containing this change and run `apm install`. The existing `fix-pr/scripts/diagnose.sh` entry point probes the Go command's help before forwarding arguments, output, and status. An absent binary or unsupported subcommand uses the frozen `diagnose-legacy.sh` throughout the N-2 window. A supported invocation that fails never falls back. The frozen fallback retains the old shell behavior, including its successful result on unreadable or empty checks. Install a supporting binary to get the inconclusive-data refusal.

## Pull request watching

Pin the harness package containing this change and run `apm install`. The `watch-pr/scripts/watch.sh` entry point probes `agentrepo checks watch --help` for the watch usage text before forwarding arguments, output, and status. Install a supporting `agentrepo` binary to use Go watching. An absent binary or unsupported subcommand runs the unchanged `watch-checks.sh` throughout the N-2 window, and consumers that call `watch-checks.sh` by name keep the shell behavior. A supported invocation returning 1, 2, or 76 never falls back.

Existing consumer settings and invocation syntax keep working. Fallback retirement waits for [binary delivery (#333)](https://github.com/tbhb-dev/devtools/issues/333) and [consumer installation (#544)](https://github.com/tbhb-dev/devtools/issues/544).

## Skill token measurement task

The repository's `skill-tokens` task runs `go run ./cmd/agentrepo skill-tokens` from the checkout. Arguments, environment settings and token records keep their existing format. Consumers don't need to change anything or upgrade an installed binary, because nothing in the tracked tree calls the internal `tools/skill-tokens.sh` file. The frozen shell and comparison fixtures remain until [#951](https://github.com/tbhb-dev/devtools/issues/951).
