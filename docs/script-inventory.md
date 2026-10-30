# Shell migration inventory and owners

This inventory tracks [#588](https://github.com/tbhb-dev/devtools/issues/588) and [#754](https://github.com/tbhb-dev/devtools/issues/754). It describes the maintained sources at `21ff3fe5`, measured on 2026-10-30.

## Scope and counting

Include shell scripts and Python helpers under `packages/agents/source/`, `.devtools/`, `tools/`, `.claude/skills/*/scripts`, `scripts/` and `tests/`. Exclude generated package and deployed skill copies when canonical sources exist, vendored dependencies, fixtures, Bats suites, and Python test suites. Include the content-post scanner because it enforces posting rules, and include the Bash test-support helpers. The inline task bodies aren't files in this inventory.

Each maintained file appears once. Each issue covers the next migration slice. Shared libraries can keep functions for later work. Use the epic for files it explicitly lists without a narrower issue. Python helpers outside migration scope name #754 and give the exclusion reason.

Lines count newline characters, including blanks and comments, as `wc -l` does. An unterminated final line isn't counted: `tools/mise-bootstrap` has 401 newlines and 402 visible lines. This differs from the source-line metric produced by `tools/script_lines.py`, which excludes blanks and full-line comments.

A `thin` wrapper only sets execution context and forwards all arguments with `exec`. Argument validation, probes, fallbacks, branches and algorithms mean `logic`, even when part of the behavior already runs in Go. Go references identify implemented behavior, with partial counterparts described explicitly. Related infrastructure alone isn't a replacement. `none` means no matching Go code for the behavior described, with the reason recorded in that row. The `pytest` references identify actual coverage of the Python helper. Shell rows use `n/a`.

Run `uv run pytest tests/tools/test_script_inventory.py` to check the real tree and negative fixtures for missing, duplicate and stale rows, owner syntax, counts, classification, and references. Ownership scope, behavior equivalence, and test coverage relationships still require independent review for AC5.

## Maintained files

| File | Owner | Newlines | Kind | Go counterpart | `pytest` coverage | Scope and evidence |
| --- | --- | ---: | --- | --- | --- | --- |
| `.claude/skills/release/scripts/guard-release.sh` | #588 | 190 | logic | none | n/a | none: `release/guard-release` restricts release actions in Bash. |
| `.claude/skills/release/scripts/release-clone.sh` | #588 | 246 | logic | none | n/a | none: `release/release-clone` creates the release checkout in Bash. |
| `.claude/skills/release/scripts/watch-release.sh` | #588 | 192 | logic | none | n/a | none: `release/watch-release` tracks release runs in Bash. |
| `.claude/skills/review-release-version/scripts/context.sh` | #588 | 181 | logic | none | n/a | none: `review-release-version/context` gathers workflow context in Bash. |
| `.devtools/scripts/apm-sync.sh` | #588 | 22 | logic | none | n/a | none: `payload/apm-sync` synchronizes hook registrations in Bash. |
| `.devtools/scripts/apm_audit.py` | #754 | 139 | logic | none | none | Excluded from migration: APM audit reporting stays Python. none: no `pytest` directly exercises this deployed copy. |
| `.devtools/scripts/check-leaks.sh` | #588 | 881 | logic | none | n/a | none: `payload/check-leaks` scans Git content for sensitive strings in Bash. |
| `.devtools/scripts/check_curl_fail.py` | #754 | 152 | logic | none | `tests/devtools/test_check_curl_fail.py` | Excluded from migration: curl option analysis stays Python. none: `check_curl_fail` stays Python. |
| `.devtools/scripts/filter-apm-deployed.py` | #754 | 34 | logic | none | none | Excluded from migration: APM file-list filtering stays Python. none: no direct Python coverage for this filter. |
| `.devtools/scripts/filter-govulncheck-sarif.sh` | #588 | 8 | logic | none | n/a | none: `payload/filter-govulncheck-sarif` filters vulnerability output in Bash. |
| `.devtools/scripts/fix_prose_replacements.py` | #754 | 256 | logic | none | none | Excluded from migration: prose replacement stays Python. none: `pytest` targets the repository copy, not this deployed copy. |
| `.devtools/scripts/fuzz.sh` | #588 | 12 | logic | `internal/agentrepo/fuzz.go` | n/a | Partial: native fuzz execution uses Go. Bash retains capability detection and the N-2 fallback. |
| `.devtools/scripts/go-ldflags.sh` | #588 | 31 | logic | none | n/a | none: `payload/go-ldflags` builds version flags in Bash. |
| `.devtools/scripts/gomodscan.sh` | #588 | 62 | logic | none | n/a | none: `payload/gomodscan` scans module dependencies in Bash. |
| `.devtools/scripts/install-pre-push.sh` | #588 | 119 | logic | `internal/commitlint/git.go` | n/a | Partial: outgoing commit checks. installer and input replay stay shell. |
| `.devtools/scripts/leak-private-strings.py` | #754 | 149 | logic | none | none | Excluded from migration: private-list resolution stays Python. none: the Bats suite `.devtools/scripts/check-leaks.bats` covers this helper. No `pytest` suite covers it. |
| `.devtools/scripts/leak-skip.sh` | #588 | 16 | logic | none | n/a | none: `payload/leak-skip` reads scanner exclusions in Bash. |
| `.devtools/scripts/legacy/fuzz.sh` | #754 | 32 | logic | `internal/agentrepo/fuzz.go` | n/a | Excluded from migration: frozen N-2 fallback for the migrated fuzz runner. Go runs the native fuzz targets. |
| `.devtools/scripts/lint-commit-msg.sh` | #588 | 80 | logic | `internal/commitlint/identities.go` | n/a | Partial: sign-off validation. preview orchestration stays shell. |
| `.devtools/scripts/lint-draft.sh` | #717 | 120 | logic | none | n/a | none: `payload/lint-draft` sequences prose tools in Bash. |
| `.devtools/scripts/lint-go-arch.py` | #754 | 60 | logic | none | none | Excluded from migration: architecture exclusion configuration stays Python. none: architecture filtering lacks Python tests. |
| `.devtools/scripts/lint-types.sh` | #588 | 40 | logic | none | n/a | none: `payload/lint-types` selects Python typing roots in Bash. |
| `.devtools/scripts/preapproval.sh` | #588 | 48 | logic | none | n/a | none: `payload/preapproval` reads confirmation scopes in Bash. |
| `.devtools/scripts/rust.sh` | #588 | 97 | logic | none | n/a | none: `payload/rust` orchestrates Rust tasks in Bash. |
| `packages/agents/source/lib/agent.sh` | #588 | 173 | logic | `internal/config/repository.go` | n/a | Partial: repository configuration. review bounds and reporting stay shell. |
| `packages/agents/source/lib/gates.py` | #713 | 117 | logic | none | none | Pass-cache fingerprinting and gate execution. none: gate execution has no Go replacement. No `pytest` suite covers this cache helper. |
| `packages/agents/source/lib/lib.sh` | #588 | 917 | logic | `internal/config/config.go`. `internal/review/verdict.go`. `internal/review/run.go`. `internal/commitlint/identities.go` | n/a | Partial: configuration, verdicts, reviewer execution and sign-off checks. remaining shared orchestration stays under the epic. |
| `packages/agents/source/lib/signing.sh` | #764 | 93 | logic | none | n/a | none: verification of landed commits stays shell. Go provides signing without commit-state verification. |
| `packages/agents/source/lib/worktree-ready.sh` | #755 | 42 | logic | none | n/a | none: `repository/worktree-ready` worktree ready in Bash. |
| `packages/agents/source/skills/agent-commit/shared/scripts/commit.sh` | #588 | 6 | thin | none | n/a | none: `agent-commit/commit` commits reviewed changes in Bash. |
| `packages/agents/source/skills/agent-commit/shared/scripts/preflight.sh` | #588 | 6 | thin | none | n/a | none: `agent-commit/preflight` checks prerequisites in Bash. |
| `packages/agents/source/skills/agent-commit/shared/scripts/review.sh` | #588 | 6 | thin | none | n/a | none: `agent-commit/review` invokes the independent reviewer in Bash. |
| `packages/agents/source/skills/agent-merge-pr/shared/scripts/lint-squash-msg.sh` | #588 | 5 | thin | none | n/a | none: `agent-merge-pr/lint-squash-msg` runs message lint in Bash. |
| `packages/agents/source/skills/agent-merge-pr/shared/scripts/merge.sh` | #588 | 11 | logic | none | n/a | none: `agent-merge-pr/merge` dispatches squash merging in Bash. |
| `packages/agents/source/skills/agent-merge-pr/shared/scripts/preflight.sh` | #588 | 5 | thin | none | n/a | none: `agent-merge-pr/preflight` checks prerequisites in Bash. |
| `packages/agents/source/skills/agent-merge-pr/shared/scripts/review.sh` | #588 | 5 | thin | none | n/a | none: the shell sends the squash message to an independent reader. |
| `packages/agents/source/skills/agent-merge-pr/shared/scripts/squash-message.sh` | #588 | 5 | thin | none | n/a | none: `agent-merge-pr/squash-message` prepares the squash draft in Bash. |
| `packages/agents/source/skills/agent-pr/shared/scripts/check.sh` | #588 | 5 | thin | none | n/a | none: `agent-pr/check` checks publication inputs in Bash. |
| `packages/agents/source/skills/agent-pr/shared/scripts/preflight.sh` | #588 | 5 | thin | none | n/a | none: `agent-pr/preflight` checks prerequisites in Bash. |
| `packages/agents/source/skills/agent-pr/shared/scripts/publish.sh` | #588 | 5 | thin | none | n/a | none: `agent-pr/publish` publishes the reviewed description in Bash. |
| `packages/agents/source/skills/agent-pr/shared/scripts/review.sh` | #588 | 5 | thin | none | n/a | none: shell dispatch connects the PR description review flow. |
| `packages/agents/source/skills/analyze-session/shared/scripts/new-retro.sh` | #588 | 159 | logic | none | n/a | none: `analyze-session/new-retro` creates a retrospective draft in Bash. |
| `packages/agents/source/skills/analyze-session/shared/scripts/preflight.sh` | #588 | 162 | logic | none | n/a | none: `analyze-session/preflight` checks prerequisites in Bash. |
| `packages/agents/source/skills/analyze-session/shared/scripts/session.py` | #754 | 885 | logic | none | none | Excluded from migration: transcript analysis stays Python. none: transcript reporting has shell integration tests only. |
| `packages/agents/source/skills/commit/claude/scripts/guard-git.sh` | #719 | 186 | logic | none | n/a | none: `commit/guard-git` restricts Git writes in Bash. |
| `packages/agents/source/skills/commit/shared/scripts/commit.sh` | #588 | 325 | logic | none | n/a | none: `commit/commit` commits reviewed changes in Bash. |
| `packages/agents/source/skills/commit/shared/scripts/preflight.sh` | #588 | 338 | logic | `internal/commitlint/identities.go` | n/a | Partial: sign-off validation via the shared library. prerequisite checks and reporting stay shell. |
| `packages/agents/source/skills/commit/shared/scripts/review.sh` | #588 | 163 | logic | `internal/review/run.go`. `internal/review/verdict.go` | n/a | Partial: the Go code runs the reviewer and parses its verdict through the shared library. shell snapshots the staged tree and binds the commit stamp. |
| `packages/agents/source/skills/fix-pr/claude/scripts/guard-fix.sh` | #588 | 79 | logic | none | n/a | none: `fix-pr/guard-fix` restricts remediation writes in Bash. |
| `packages/agents/source/skills/fix-pr/shared/scripts/diagnose.sh` | #588 | 162 | logic | `internal/agentrepo/diagnose.go` | n/a | Go reports failing checks and local reproducers. the shell entry point remains available. |
| `packages/agents/source/skills/fix-pr/shared/scripts/populate-description.sh` | #588 | 108 | logic | none | n/a | none: the shell copies the existing PR body for remediation. |
| `packages/agents/source/skills/fix-prose/claude/scripts/arm-guard.sh` | #588 | 74 | logic | none | n/a | none: `fix-prose/arm-guard` records the target file in Bash. |
| `packages/agents/source/skills/fix-prose/claude/scripts/guard-target.sh` | #588 | 130 | logic | none | n/a | none: `fix-prose/guard-target` confines edits to that target in Bash. |
| `packages/agents/source/skills/fix-prose/claude/scripts/release-once.sh` | #588 | 73 | logic | none | n/a | none: `fix-prose/release-once` permits a scoped edit in Bash. |
| `packages/agents/source/skills/fix-prose/shared/scripts/check-suppressions.sh` | #588 | 393 | logic | none | n/a | none: `fix-prose/check-suppressions` checks prose rule exceptions in Bash. |
| `packages/agents/source/skills/fix-prose/shared/scripts/preflight.sh` | #588 | 129 | logic | none | n/a | none: `fix-prose/preflight` checks prerequisites in Bash. |
| `packages/agents/source/skills/merge-pr/claude/scripts/guard-merge.sh` | #721 | 75 | logic | none | n/a | none: `merge-pr/guard-merge` restricts merge commands in Bash. |
| `packages/agents/source/skills/merge-pr/shared/scripts/lint-squash-msg.sh` | #588 | 37 | logic | none | n/a | none: `merge-pr/lint-squash-msg` runs message lint in Bash. |
| `packages/agents/source/skills/merge-pr/shared/scripts/merge-agent.sh` | #761 | 224 | logic | `internal/checks/checks.go`. `internal/config/repository.go`. `internal/ghpost/run.go`. `internal/mergepolicy/approval.go` | n/a | Partial: check evaluation, exact-head approval, repository configuration, and scanned posting. shell coordinates merge policy. |
| `packages/agents/source/skills/merge-pr/shared/scripts/merge-policy.py` | #588 | 816 | logic | none | none | Merge policy inspection. none: check evaluation is separate behavior, not a policy counterpart. Bats exercises policy behavior. No Python suite covers this helper. |
| `packages/agents/source/skills/merge-pr/shared/scripts/populate-description.sh` | #588 | 108 | logic | none | n/a | none: shell extracts briefing material before a squash. |
| `packages/agents/source/skills/merge-pr/shared/scripts/preflight.sh` | #723 | 224 | logic | none | n/a | none: `merge-pr/preflight` checks prerequisites in Bash. |
| `packages/agents/source/skills/merge-pr/shared/scripts/pull-request.sh` | #723 | 165 | logic | none | n/a | none: `merge-pr/pull-request` reads pinned pull request details in Bash. |
| `packages/agents/source/skills/merge-pr/shared/scripts/review.sh` | #588 | 154 | logic | `internal/review/run.go`. `internal/review/verdict.go` | n/a | Partial: the shared library uses Go to control the process and interpret the verdict. Bash gathers the pinned merge inputs and records approval evidence. |
| `packages/agents/source/skills/merge-pr/shared/scripts/squash-merge.sh` | #723 | 376 | logic | `internal/commitlint/message.go`. `internal/ghpost/run.go` | n/a | Partial: message rules and scanned posting. owner covers check-judge caller only. the epic retains merge execution. |
| `packages/agents/source/skills/merge-pr/shared/scripts/squash-message.sh` | #588 | 278 | logic | `internal/commitlint/identities.go` | n/a | Partial: sign-off validation. briefing stays shell. |
| `packages/agents/source/skills/pr/claude/scripts/guard-gh.sh` | #719 | 77 | logic | none | n/a | none: `pr/guard-gh` restricts GitHub verbs in Bash. |
| `packages/agents/source/skills/pr/shared/scripts/check.sh` | #588 | 12 | logic | none | n/a | none: publication input checks use a shell dispatcher. |
| `packages/agents/source/skills/pr/shared/scripts/create-pr.sh` | #588 | 209 | logic | `internal/ghpost/run.go` | n/a | Partial: scanned posting. gates and push orchestration stay shell. |
| `packages/agents/source/skills/pr/shared/scripts/description.sh` | #588 | 109 | logic | none | n/a | none: `pr/description` reads draft fields in Bash. |
| `packages/agents/source/skills/pr/shared/scripts/pr-agent.sh` | #588 | 99 | logic | none | n/a | none: `pr/pr-agent` checks unattended publication authority in Bash. |
| `packages/agents/source/skills/pr/shared/scripts/pr-checks.sh` | #588 | 54 | logic | none | n/a | none: `pr/pr-checks` checks description policy in Bash. |
| `packages/agents/source/skills/pr/shared/scripts/preflight.sh` | #588 | 404 | logic | none | n/a | none: `pr/preflight` checks prerequisites in Bash. |
| `packages/agents/source/skills/pr/shared/scripts/properties.sh` | #588 | 64 | logic | none | n/a | none: `pr/properties` resolves publication properties in Bash. |
| `packages/agents/source/skills/pr/shared/scripts/publish.sh` | #588 | 226 | logic | `internal/ghpost/run.go` | n/a | Partial: scanned content publication. properties and Git orchestration stay shell. |
| `packages/agents/source/skills/pr/shared/scripts/review.sh` | #588 | 134 | logic | `internal/review/run.go`. `internal/review/verdict.go` | n/a | Partial: reviewer invocation and verdict handling use Go via the library. description snapshots and publication stamps remain in Bash. |
| `packages/agents/source/skills/pr/shared/scripts/scaffold-description.sh` | #588 | 91 | logic | none | n/a | none: `pr/scaffold-description` writes a template scaffold in Bash. |
| `packages/agents/source/skills/pr/shared/scripts/validate-description.sh` | #718 | 553 | logic | none | n/a | none: `pr/validate-description` checks the draft structure in Bash. |
| `packages/agents/source/skills/rebase/claude/scripts/guard-rebase.sh` | #722 | 194 | logic | none | n/a | none: `rebase/guard-rebase` rejects unsafe replay commands in Bash. |
| `packages/agents/source/skills/rebase/shared/scripts/continue-rebase.sh` | #588 | 169 | logic | none | n/a | none: `rebase/continue-rebase` checks staged changes before continuing in Bash. |
| `packages/agents/source/skills/rebase/shared/scripts/marker-scan.sh` | #714 | 97 | logic | `internal/agentrepo/markers.go` | n/a | Marker scanning implemented. probe and frozen fallback still hold logic. |
| `packages/agents/source/skills/rebase/shared/scripts/preflight.sh` | #588 | 340 | logic | none | n/a | none: `rebase/preflight` checks prerequisites in Bash. |
| `packages/agents/source/skills/rebase/shared/scripts/rebase-status.sh` | #588 | 157 | logic | none | n/a | none: `rebase/rebase-status` reports a stopped replay in Bash. |
| `packages/agents/source/skills/rebase/shared/scripts/start-rebase.sh` | #588 | 165 | logic | none | n/a | none: `rebase/start-rebase` starts the pinned replay in Bash. |
| `packages/agents/source/skills/rebase/shared/scripts/verify-rebase.sh` | #588 | 269 | logic | none | n/a | none: `rebase/verify-rebase` compares the replay result in Bash. |
| `packages/agents/source/skills/resolve-rebase-conflicts/claude/scripts/guard-resolve.sh` | #722 | 161 | logic | none | n/a | none: `resolve-rebase-conflicts/guard-resolve` blocks side selection and marker staging in Bash. |
| `packages/agents/source/skills/resolve-rebase-conflicts/shared/scripts/classify-conflicts.sh` | #588 | 193 | logic | none | n/a | none: `resolve-rebase-conflicts/classify-conflicts` groups conflicted paths in Bash. |
| `packages/agents/source/skills/resolve-rebase-conflicts/shared/scripts/conflict_shape.py` | #754 | 278 | logic | none | `tests/skills/resolve-rebase-conflicts/scripts/test_conflict_shape.py` | Excluded from migration: conflict shape analysis stays Python. shell migration covers its callers. none: `conflict_shape` stays Python. |
| `packages/agents/source/skills/resolve-rebase-conflicts/shared/scripts/resolve-union.sh` | #588 | 184 | logic | none | n/a | none: `resolve-rebase-conflicts/resolve-union` merges sorted append-only data in Bash. |
| `packages/agents/source/skills/resolve-rebase-conflicts/shared/scripts/take-side.sh` | #588 | 115 | logic | none | n/a | none: `resolve-rebase-conflicts/take-side` selects a whole-file resolution in Bash. |
| `packages/agents/source/skills/watch-pr/claude/scripts/guard-watch.sh` | #588 | 65 | logic | none | n/a | none: `watch-pr/guard-watch` restricts watcher commands in Bash. |
| `packages/agents/source/skills/watch-pr/shared/scripts/watch-checks.sh` | #588 | 187 | logic | none | n/a | none: `watch-pr/watch-checks` streams check transitions in Bash. |
| `packages/agents/source/skills/write-pr-description/shared/scripts/context.sh` | #588 | 172 | logic | none | n/a | none: `write-pr-description/context` gathers workflow context in Bash. |
| `packages/agents/source/skills/write-prose-fix/shared/scripts/context.sh` | #588 | 150 | logic | none | n/a | none: `write-prose-fix/context` gathers workflow context in Bash. |
| `scripts/clear-commit-agentmsg.sh` | #715 | 27 | logic | none | n/a | none: `repository/clear-commit-agentmsg` deletes the used message draft in Bash. |
| `scripts/cspell-commit-msg.sh` | #715 | 52 | logic | none | n/a | none: `repository/cspell-commit-msg` checks message spelling in Bash. |
| `scripts/vale-commit-msg.sh` | #715 | 80 | logic | none | n/a | none: `repository/vale-commit-msg` checks message prose in Bash. |
| `tests/skills/agent-merge-pr/test_helper.bash` | #334 | 61 | logic | none | n/a | none: fixture setup for `tests/skills/agent-merge-pr` uses Bash. |
| `tests/skills/agent-pr/test_helper.bash` | #334 | 24 | logic | none | n/a | none: fixture setup for `tests/skills/agent-pr` uses Bash. |
| `tests/skills/merge-pr/gh-merge.bash` | #334 | 55 | logic | none | n/a | none: `merge-pr/gh-merge` simulates GitHub merging in Bash. |
| `tests/skills/merge-pr/test_helper.bash` | #334 | 213 | logic | none | n/a | none: fixture setup for `tests/skills/merge-pr` uses Bash. |
| `tests/skills/pr/test_helper.bash` | #334 | 154 | logic | none | n/a | none: fixture setup for `tests/skills/pr` uses Bash. |
| `tests/skills/redesign.bash` | #334 | 90 | logic | none | n/a | none: `redesign.bash/redesign` exercises review redesign states in Bash. |
| `tests/skills/test_content_posts.py` | #757 | 126 | logic | none | `tests/skills/test_content_posts.py` | The scanner uses `pytest` assertions. none: no Go bypass scanner yet. |
| `tests/test_helper.bash` | #334 | 30 | logic | none | n/a | none: fixture setup for `tests` uses Bash. |
| `tools/agy_otel_latency.py` | #754 | 80 | logic | none | `tests/tools/test_agy_otel_latency.py` | Excluded from migration: telemetry analysis stays Python. none: `agy_otel_latency` stays Python. |
| `tools/apm_audit.py` | #754 | 139 | logic | none | `tests/tools/test_apm_audit.py` | Excluded from migration: the Python audit reporter analyzes deployment findings. none: `apm_audit` stays Python. |
| `tools/check-script-hygiene.sh` | #588 | 172 | logic | none | n/a | none: `repository/check-script-hygiene` checks shell hazards in Bash. |
| `tools/check-versions.sh` | #716 | 4 | thin | `internal/versions/check.go` | n/a | Go checks release literals and local locks. |
| `tools/fix_prose_replacements.py` | #754 | 256 | logic | none | `tests/tools/test_fix_prose_replacements.py` | Excluded from migration: the helper applies span-checked prose corrections. none: `fix_prose_replacements` stays Python. |
| `tools/fuzz.sh` | #588 | 4 | thin | `internal/agentrepo/fuzz.go` | n/a | The shell forwards arguments to the payload adapter for the Go fuzz runner. |
| `tools/generate-skills.sh` | #588 | 172 | logic | `internal/skills/generate.go` | n/a | Go renders and checks skill output. the shell generator remains the current entry point. |
| `tools/go-ldflags.sh` | #588 | 4 | thin | none | n/a | none: `repository/go-ldflags` builds version flags in Bash. |
| `tools/lint-draft.sh` | #717 | 120 | logic | none | n/a | none: a duplicate shell copy coordinates draft lint. |
| `tools/lint-types.sh` | #588 | 4 | thin | none | n/a | none: `repository/lint-types` selects Python typing roots in Bash. |
| `tools/mise-bootstrap` | #588 | 401 | logic | none | n/a | none: `repository/mise-bootstrap` installs the pinned task runner in Bash. |
| `tools/release-dispatch.sh` | #588 | 82 | logic | none | n/a | none: `repository/release-dispatch` requests the release workflow in Bash. |
| `tools/release-readiness.sh` | #588 | 183 | logic | none | n/a | none: `repository/release-readiness` checks release prerequisites in Bash. |
| `tools/release-verify.sh` | #588 | 237 | logic | none | n/a | none: `repository/release-verify` verifies release artifacts in Bash. |
| `tools/script_lines.py` | #754 | 37 | logic | none | `tests/tools/test_script_lines.py` | Excluded from migration: source-line measurement stays Python. none: `script_lines` stays Python. |
| `tools/skill-limits.sh` | #588 | 154 | logic | none | n/a | none: `repository/skill-limits` checks platform manifest bounds in Bash. |
| `tools/skill-tokens.sh` | #588 | 284 | logic | `internal/tokens/measure.go` | n/a | Go measures manifest tokens deterministically. Bash retains its original measurement command. |
| `tools/skill_attribution.py` | #754 | 555 | logic | none | `tests/tools/test_skill_attribution.py` | Excluded from migration: transcript attribution analysis stays Python. none: `skill_attribution` stays Python. |
| `tools/test-guards.sh` | #588 | 242 | logic | none | n/a | none: `repository/test-guards` exercises registered guard commands in Bash. |

Totals: 108 shell files (7 Bash test helpers included), 15292 shell newlines. 16 Python helpers.

## Historical timing baseline

An earlier inventory measured `f07e183a04dee24b9d7bc18f0135791f8960ef47` on 2026-10-18, before the ports. Keep that baseline for #334's before/after comparison: median `mise run test` time was 92.18 seconds, median Bats time 89.89 seconds, and median per-run Bats share 97.5%. They measure that earlier tree.

Timing ran on Ubuntu on WSL2 with Linux `6.6.87.2-microsoft-standard-WSL2`, x86_64, Intel Core i9-13900KF, with 32 logical processors. Other agents shared the host without a reservation. Timing used Python's monotonic clock around sequential `mise run test` invocations with normal caches. Measurements omit exploratory warm-up and an overlapping run. Bats included `test-hooks`, `test-leaks`, startup, and cleanup.

| Run | Total seconds | Bats seconds | Bats share | Start load (1/5/15) | End load | Highest sampled load | Result |
| --- | ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 91.55 | 89.18 | 97.4% | 6.61/2.54/0.97 | 4.12/2.72/1.17 | 6.65/2.72/1.17 | Pass |
| 2 | 92.18 | 89.89 | 97.5% | 4.12/2.72/1.17 | 4.18/3.20/1.50 | 4.87/3.22/1.50 | Pass |
| 3 | 92.91 | 90.68 | 97.6% | 4.18/3.20/1.50 | 3.67/3.24/1.69 | 4.82/3.37/1.69 | Pass |
