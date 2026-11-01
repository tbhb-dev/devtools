# Script inventory and test baseline

Related #203. This first slice records the baseline and proposed destinations. Ports, the gate at 200 lines of source code, and consumer follow-ups remain later work. Command ownership follows the proposals in [#174](https://github.com/tbhb-dev/devtools/issues/174), pending that issue’s CLI decisions.

## Scope and counting

Snapshot: `f07e183a04dee24b9d7bc18f0135791f8960ef47` on `origin/main`, measured on 2026-10-18. The inventory excludes generated package and deployed skill copies because they follow their source. It includes every first-party Bats suite and the Bash bootstrap without a file extension. It excludes vendored dependencies. Count source lines in each file with `uv run tools/script_lines.py PATH ...`. The tested counter excludes blank lines and lines whose first non-whitespace character is `#`, including shebangs. Comments after code leave that line counted. Continuations, here-document data, and embedded languages use the same physical-line rule.

This reproducible estimate measures size. The counter neither parses the code nor counts executable statements, and this change doesn't install a size gate.

Tasks defined in the configuration qualify when their combined run bodies exceed three source lines of code, including arrays of commands. Each array entry contributes its physical code lines. Counts use decoded strings in Tom's Obvious Minimal Language (TOML). They exclude the format's delimiters and task metadata. The inventory omits bodies with three or fewer source lines of code.

The coverage column identifies suites rather than percentages. A suite may exercise a generated copy or a helper through its parent workflow. “None” means no behavioral Bats coverage identified. Static lint isn't behavioral coverage. B identifiers resolve to measured suite wall times below and don't assign that entire time to each covered script.

## Machine and baseline

Shared host running Ubuntu on WSL2, Linux `6.6.87.2-microsoft-standard-WSL2`, x86_64, Intel Core i9-13900KF, 32 logical processors. Other agents share this machine. The machine wasn't idle during these runs and has no reservation for this work. Load samples are the Linux 1/5/15-minute averages. These don't measure Windows host activity.

Timing uses Python `time.monotonic()` around `mise run test`, with merged stdout/stderr streamed to a log. Stage boundaries are the combined test task’s `mise run` announcements. Bats time includes `test-hooks` and `test-leaks` along with task startup and cleanup. Runs are sequential, use normal caches, and don't clear Go or tool caches. The measurements exclude an exploratory warm-up and the first timed run that overlapped it. The recorded runs include the new counter tests but no script ports. Suite timings are separate sequential `mise exec -- bats PATH` invocations, one run per suite, including runner startup.

| Run | Total seconds | Bats seconds | Bats share | Start load (1/5/15) | End load | Highest sampled load | Result |
| --- | ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 91.55 | 89.18 | 97.4% | 6.61/2.54/0.97 | 4.12/2.72/1.17 | 6.65/2.72/1.17 | Pass |
| 2 | 92.18 | 89.89 | 97.5% | 4.12/2.72/1.17 | 4.18/3.20/1.50 | 4.87/3.22/1.50 | Pass |
| 3 | 92.91 | 90.68 | 97.6% | 4.18/3.20/1.50 | 3.67/3.24/1.69 | 4.82/3.37/1.69 | Pass |

Median total: 92.18 seconds. Median Bats time: 89.89 seconds. Median per-run Bats share: 97.5%. These measurements precede the ports, so this change doesn't claim a speedup.

## Source scripts by area, ranked by source lines of code

Decision keys: G0 means Go shared internals under #174. G1 means a Go command in [#174's repository transaction binary](https://github.com/tbhb-dev/devtools/issues/174), G2 its `agentcontext` binary, G3 a standalone guard for #174 hooks, and G4 a Go command outside #174. G7 means a Go command in #174's `agentstore` binary. G5 means Go unit and integration tests under #174, while G6 means other Go tests. P means a Python script and P1 means Python tests using `pytest`. B means Bash command glue. This document proposes destinations rather than new public interfaces. Bash rows keep thin adapters. Improve efficiency or extract code where noted.

### Shared workflow library

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `packages/agents/source/lib/lib.sh` | 476 | All generated workflow skills. consumer | B18, B17, B19 | G0: configuration authority, review stamps, quota, and gate-cache logic need portable libraries. |

### Shared payload scripts

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `.devtools/scripts/check-leaks.sh` | 402 | devtools:check-leaks task and pre-push hook. consumer. CI | B42 | G1: the Git object traversal and pattern policy need portable parsing. |
| `.devtools/scripts/install-pre-push.sh` | 68 | devtools:install-pre-push-hook and compatibility leak tasks. consumer | B42 | G1: hook installation edits Git state and preserves input. |
| `.devtools/scripts/preapproval.sh` | 44 | preapprove, revoke, and check tasks. consumer | B06 | G7: scope validation and grants need one state contract. |
| `.devtools/scripts/lint-draft.sh` | 42 | devtools:lint-draft task. consumer | B22 | B: sequence external prose gates and keep their statuses. |
| `.devtools/scripts/apm-sync.sh` | 21 | devtools:apm-sync task. consumer | B33 | G1: read and update hook JSON without shell pipelines. |
| `.devtools/scripts/lint-commit-msg.sh` | 18 | lint-commit-msg task. consumer | B02 | G1: preview identity and trailers without shell text parsing. |
| `.devtools/scripts/lint-types.sh` | 18 | devtools:lint-types task. consumer | None | P: path discovery and worktree filtering fit the standard library. |
| `.devtools/scripts/fuzz.sh` | 17 | devtools:fuzz-go task. consumer | None | G4: discover fuzz targets and sequence subprocesses portably. |
| `.devtools/scripts/leak-skip.sh` | 13 | check-leaks working-tree scan. consumer | B42 | G1: share skip-policy parsing with the scanner. |
| `.devtools/scripts/go-ldflags.sh` | 10 | devtools:build and install tasks. consumer | B26, B33 | G4: the Git metadata and flag quoting need portable behavior. |
| `.devtools/scripts/filter-govulncheck-sarif.sh` | 6 | scan-govulncheck workflow. consumer. CI | B39 | B: thin `jq` invocation with an input/output refusal. |

### Pull request workflow scripts

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `packages/agents/source/skills/pr/shared/scripts/validate-description.sh` | 376 | pr skill/workflow. consumer | B14 | G1: parse draft structure with typed publication contracts. |
| `packages/agents/source/skills/pr/shared/scripts/preflight.sh` | 285 | pr skill/workflow. consumer | B14 | G1: check typed draft and GitHub publication prerequisites. |
| `packages/agents/source/skills/pr/shared/scripts/publish.sh` | 165 | pr skill/workflow. consumer | B14 | G1: publish typed draft data through GitHub contracts. |
| `packages/agents/source/skills/pr/shared/scripts/create-pr.sh` | 100 | pr skill/workflow. consumer | B14 | G1: apply typed publication contracts to draft creation. |
| `packages/agents/source/skills/pr/shared/scripts/review.sh` | 80 | pr skill/workflow. consumer | B14 | G1: review typed publication drafts against GitHub state. |
| `packages/agents/source/skills/pr/shared/scripts/description.sh` | 58 | pr skill/workflow. consumer | B14 | G1: parse the publication draft through typed contracts. |
| `packages/agents/source/skills/pr/shared/scripts/scaffold-description.sh` | 48 | pr skill/workflow. consumer | B14 | G1: create a draft that follows the publication contract. |
| `packages/agents/source/skills/pr/shared/scripts/properties.sh` | 47 | pr skill/workflow. consumer | B14 | G1: parse typed properties before GitHub publication. |
| `packages/agents/source/skills/pr/claude/scripts/guard-gh.sh` | 28 | pr hook. consumer | None | G3: test refused `gh` commands through portable parsers. |

### Repository shell adapters

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `tools/mise-bootstrap` | 344 | Developer bootstrap CLI and Renovate update recipe | B33 | B: replace the copied installer with thin pinned bootstrap glue below 200 lines of source code. |
| `tools/lint-draft.sh` | 42 | lint-draft task | B22 | B: forward to the shared payload code without duplicating its logic. |
| `tools/release-dispatch.sh` | 35 | release-devtools task. CI | B34 | B: thin validated dispatch wrapper. keep real workflow dispatch outside tests. |
| `tools/fuzz.sh` | 2 | Legacy forwarding CLI | None | B: forward to the shared payload code without duplicating its logic. |
| `tools/go-ldflags.sh` | 2 | run task | B26, B33 | B: forward to the shared payload code without duplicating its logic. |
| `tools/lint-types.sh` | 2 | Legacy forwarding CLI | None | B: delegate type checks to the existing payload. |

### Workflow scripts: fix-prose

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `packages/agents/source/skills/fix-prose/shared/scripts/check-suppressions.sh` | 263 | fix-prose skill/workflow. consumer | B12 | G1: parse suppression records and run state portably. |
| `packages/agents/source/skills/fix-prose/shared/scripts/preflight.sh` | 72 | fix-prose skill/workflow. consumer | B12 | G1: check suppression state before a prose run. |
| `packages/agents/source/skills/fix-prose/claude/scripts/guard-target.sh` | 46 | fix-prose hook. consumer | B12 | G3: test target restrictions with portable command parsing. |
| `packages/agents/source/skills/fix-prose/claude/scripts/release-once.sh` | 29 | fix-prose hook. consumer | B12 | G3: test one-time releases against portable hook payload readers. |
| `packages/agents/source/skills/fix-prose/claude/scripts/arm-guard.sh` | 19 | fix-prose hook. consumer | None | G3: test guard activation through portable payload readers. |

### Workflow scripts: rebase

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `packages/agents/source/skills/rebase/shared/scripts/preflight.sh` | 242 | rebase skill/workflow. consumer | B15 | G1: check Git replay prerequisites portably. |
| `packages/agents/source/skills/rebase/shared/scripts/verify-rebase.sh` | 184 | rebase skill/workflow. consumer | B15 | G1: compare replayed ranges through portable process control. |
| `packages/agents/source/skills/rebase/shared/scripts/continue-rebase.sh` | 115 | rebase skill/workflow. consumer | B15 | G1: continue Git replay through portable process control. |
| `packages/agents/source/skills/rebase/shared/scripts/rebase-status.sh` | 109 | rebase skill/workflow. consumer | B15 | G1: read replay status through portable process control. |
| `packages/agents/source/skills/rebase/claude/scripts/guard-rebase.sh` | 95 | rebase hook. consumer | None | G3: parse rebase commands portably and test refused replays. |
| `packages/agents/source/skills/rebase/shared/scripts/start-rebase.sh` | 93 | rebase skill/workflow. consumer | B15 | G1: launch the replay with consistent process handling. |
| `packages/agents/source/skills/rebase/shared/scripts/marker-scan.sh` | 42 | rebase skill/workflow. consumer | B15 | G1: scan replay output through portable process control. |

### Workflow scripts: commit

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `packages/agents/source/skills/commit/shared/scripts/preflight.sh` | 214 | commit skill/workflow. consumer | B11 | G1: check mutation prerequisites and review stamps. |
| `packages/agents/source/skills/commit/shared/scripts/commit.sh` | 165 | commit skill/workflow. consumer | B11 | G1: test commit mutations and review stamps as one transaction. |
| `packages/agents/source/skills/commit/shared/scripts/review.sh` | 82 | commit skill/workflow. consumer | B11 | G1: check review stamps before Git mutations. |
| `packages/agents/source/skills/commit/claude/scripts/guard-git.sh` | 76 | commit hook. consumer | None | G3: parse Git commands portably before testing refused commits. |

### Repository tools: Python destinations

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `tools/skill-tokens.sh` | 188 | frozen comparison tests | B09 | Go task active. Shell and comparison fixtures retire in [#951](https://github.com/tbhb-dev/devtools/issues/951). |
| `tools/generate-skills.sh` | 112 | generate-skills and check-generated-skills tasks. CI | B32 | P: template expansion and directory comparison fit standard-library processing. |
| `tools/skill-limits.sh` | 99 | lint-skills task | None | P: parse manifest fields and report limits in process. |
| `tools/check-versions.sh` | 85 | check-versions task. CI | B31 | P: compare release literals as text without shell regular expression differences. |
| `tools/check-script-hygiene.sh` | 82 | lint-script-hygiene task. CI | B33 | P: collect files and report structural checker results. |

### Workflow scripts: merge-pr

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `packages/agents/source/skills/merge-pr/shared/scripts/squash-merge.sh` | 183 | merge-pr skill/workflow. consumer | B13 | G1: apply the pinned merge through transaction tests. |
| `packages/agents/source/skills/merge-pr/shared/scripts/preflight.sh` | 153 | merge-pr skill/workflow. consumer | B13 | G1: check pinned merge prerequisites with transaction tests. |
| `packages/agents/source/skills/merge-pr/shared/scripts/squash-message.sh` | 131 | merge-pr skill/workflow. consumer | B13 | G1: assemble the message from pinned merge state. |
| `packages/agents/source/skills/merge-pr/shared/scripts/review.sh` | 102 | merge-pr skill/workflow. consumer | B13 | G1: review messages against pinned merge state. |
| `packages/agents/source/skills/merge-pr/shared/scripts/pull-request.sh` | 91 | merge-pr skill/workflow. consumer | B13 | G1: read the pinned pull request for merge transactions. |
| `packages/agents/source/skills/merge-pr/shared/scripts/populate-description.sh` | 62 | merge-pr skill/workflow. consumer | B13 | G1: read pinned merge context into the draft. |
| `packages/agents/source/skills/merge-pr/claude/scripts/guard-merge.sh` | 32 | merge-pr hook. consumer | None | G3: test refused merges with portable command readers. |

### Repository tools: Go destinations

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `tools/test-guards.sh` | 151 | Manual regression CLI outside combined test task | None | G3: move refusal cases to guard unit tests. Use a small shell smoke test. |
| `tools/release-verify.sh` | 116 | verify-devtools-release task. CI | B36 | G4: tag type, signature, and ancestry checks are repository logic. |
| `tools/release-readiness.sh` | 66 | check-devtools-release-readiness task. CI | B35 | G4: the Git graph and release-state checks need portable process control. |

### Workflow scripts: resolve-rebase-conflicts

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `packages/agents/source/skills/resolve-rebase-conflicts/shared/scripts/classify-conflicts.sh` | 129 | resolve-rebase-conflicts skill/workflow. consumer | B16 | G1: classify conflicting paths deterministically. |
| `packages/agents/source/skills/resolve-rebase-conflicts/shared/scripts/resolve-union.sh` | 124 | resolve-rebase-conflicts skill/workflow. consumer | B16 | G1: resolve sorted path contents deterministically. |
| `packages/agents/source/skills/resolve-rebase-conflicts/claude/scripts/guard-resolve.sh` | 89 | resolve-rebase-conflicts hook. consumer | None | G3: test refused conflict resolutions through portable payload parsing. |
| `packages/agents/source/skills/resolve-rebase-conflicts/shared/scripts/take-side.sh` | 67 | resolve-rebase-conflicts skill/workflow. consumer | None | G1: select conflict contents with deterministic path handling. |

### Workflow scripts: write-pr-description

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `packages/agents/source/skills/write-pr-description/shared/scripts/context.sh` | 122 | write-pr-description skill/workflow. consumer | B21 | G2: collect context for prose targets through portable readers. |
| `packages/agents/source/skills/write-pr-description/claude/scripts/guard-draft.sh` | 23 | write-pr-description hook. consumer | None | G3: parse draft-writing hooks portably and test prohibited commands. |

### Workflow scripts: watch-pr

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `packages/agents/source/skills/watch-pr/shared/scripts/watch-checks.sh` | 113 | watch-pr skill/workflow. consumer | B20 | G2: test check polling with a fake clock and API fixtures. |
| `packages/agents/source/skills/watch-pr/claude/scripts/guard-watch.sh` | 33 | watch-pr hook. consumer | None | G3: test monitoring restrictions against portable hook readers. |

### Workflow scripts: fix-pr

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `packages/agents/source/skills/fix-pr/shared/scripts/diagnose.sh` | 112 | fix-pr skill/workflow. consumer | None | G1: parse failing Actions state with the shared GitHub client. |
| `packages/agents/source/skills/fix-pr/shared/scripts/populate-description.sh` | 62 | fix-pr skill/workflow. consumer | None | G1: read failing Actions context through the GitHub client. |
| `packages/agents/source/skills/fix-pr/claude/scripts/guard-fix.sh` | 39 | fix-pr hook. consumer | None | G3: parse repair commands and test their refusal cases portably. |

### Hook adapters

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `scripts/commitlint.sh` | 109 | prek commit-msg hook. consumer | B08 | Full Conventional Commits linter. Migrated to `cmd/commit-lint` with golden Go tests. |
| `scripts/commit-trailers.sh` | 55 | prek commit-msg hook. consumer | B07 | Trailer checks now run in the Go package. |
| `scripts/vale-commit-msg.sh` | 12 | prek commit-msg hook. consumer | B02 | B: thin Vale adapter preserves the hook contract. |
| `scripts/cspell-commit-msg.sh` | 9 | prek commit-msg hook. consumer | None | B: thin cspell adapter preserves the hook contract. |
| `scripts/clear-commit-agentmsg.sh` | 4 | prek post-commit hook. consumer | B04 | B: remove the draft after a successful commit. |

### Workflow scripts: analyze-session

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `packages/agents/source/skills/analyze-session/shared/scripts/new-retro.sh` | 107 | analyze-session skill/workflow. consumer | B10 | P: transcript paths and retrospective reports fit standard-library processing. |
| `packages/agents/source/skills/analyze-session/shared/scripts/preflight.sh` | 99 | analyze-session skill/workflow. consumer | B10 | P: check transcript paths before offline report processing. |

### Workflow scripts: write-prose-fix

| Script | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `packages/agents/source/skills/write-prose-fix/shared/scripts/context.sh` | 101 | write-prose-fix skill/workflow. consumer | B21 | G2: read Git and draft context portably. |

Other tests may cover guards whose rows don't identify Bats coverage: `tools/test-guards.sh` exercises fix-prose guards separately and is outside the combined test task. Other guard gaps need explicit refusal fixtures before porting. The `masked-status` suites cover subprocess error propagation, not every path of a script. `tests/skills/fix-pr/scripts/test_diagnose.py` uses `pytest` to check reproducer mapping through Bash. `fix-pr` has no dedicated Bats suite in this snapshot.

## Task bodies in mise configuration

### Repository tasks: command sequences

| File and task | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `mise.toml`: `bootstrap` | 4 | Repository task. CLI/CI | None | B: sequence `bootstrap` commands. Preserve shell integration tests. |
| `mise.toml`: `run` | 5 | Repository task. CLI/CI | B33 | B: sequence `run` commands. Preserve shell integration tests. |
| `mise.toml`: `lint` | 19 | Repository task. CLI/CI | None | B: sequence `lint` commands. Preserve shell integration tests. |
| `mise.toml`: `test` | 4 | Repository task. CLI/CI | None | B: sequence `test` commands. Preserve shell integration tests. |
| `mise.toml`: `security` | 4 | Repository task. CLI/CI | None | B: sequence `security` commands. Preserve shell integration tests. |
| `mise.toml`: `check` | 6 | Repository task. CLI/CI | None | B: sequence `check` commands. Preserve shell integration tests. |
| `mise.toml`: `check-all` | 4 | Repository task. CLI/CI | None | B: sequence `check-all` commands. Preserve shell integration tests. |

### Repository tasks: reports and synchronization

| File and task | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `mise.toml`: `apm-sync` | 21 | Repository task. CLI/CI | B33 | G1: share the hook JSON update code with the payload. |
| `mise.toml`: `version` | 7 | Repository task. CLI/CI | None | P: format release text from Git metadata offline. |
| `mise.toml`: `generate-changelog` | 9 | Repository task. CLI/CI | B33 | P: turn Git metadata into changelog text offline. |
| `mise.toml`: `generate-release-notes` | 8 | Repository task. CLI/CI | B33 | P: the Git metadata and release-note text formatting fit report processing. |

### Shared Go language tasks

| File and task | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `.devtools/tasks/go.toml`: `devtools:fix-go` | 8 | Shared task. consumer CLI/CI | None | B: sequence `devtools:fix-go` commands. Preserve shell integration tests. |
| `.devtools/tasks/go.toml`: `devtools:lint-go-all` | 4 | Shared task. consumer CLI/CI | B26 | B: sequence `devtools:lint-go-all` commands. Preserve shell integration tests. |
| `.devtools/tasks/go.toml`: `devtools:lint-go-modernize` | 7 | Shared task. consumer CLI/CI | B26 | B: invoke modernization checks through shell glue. Preserve shell integration tests. |
| `.devtools/tasks/go.toml`: `devtools:lint-go-deadcode` | 7 | Shared task. consumer CLI/CI | B26 | B: run dead-code tooling as a Bash adapter. Preserve shell integration tests. |
| `.devtools/tasks/go.toml`: `devtools:lint-go-deadcode-tests` | 7 | Shared task. consumer CLI/CI | B26 | B: sequence dead-code checks that include tests. Preserve shell integration tests. |
| `.devtools/tasks/go.toml`: `devtools:cover-go` | 6 | Shared task. consumer CLI/CI | None | B: coordinate coverage tools at the shell boundary. Preserve shell integration tests. |
| `.devtools/tasks/go.toml`: `devtools:cover-go-cobertura` | 4 | Shared task. consumer CLI/CI | None | B: invoke coverage conversion from Bash. Preserve shell integration tests. |
| `.devtools/tasks/go.toml`: `devtools:cover-go-binary` | 7 | Shared task. consumer CLI/CI | None | B: run the coverage binary through command glue. Preserve shell integration tests. |
| `.devtools/tasks/go.toml`: `devtools:cover-go-merge` | 7 | Shared task. consumer CLI/CI | B33 | P: discover coverage directories without parsing ls output. |
| `.devtools/tasks/go.toml`: `devtools:gomodscan` | 7 | Shared task. consumer CLI/CI | B25 | B: sequence `devtools:gomodscan` commands. Preserve shell integration tests. |
| `.devtools/tasks/go.toml`: `devtools:gomodscan-sarif` | 4 | Shared task. consumer CLI/CI | B25 | B: sequence output from module scanners for SARIF. Preserve shell integration tests. |
| `.devtools/tasks/go.toml`: `devtools:vendor-check` | 14 | Shared task. consumer CLI/CI | B29 | G1: preserve Git drift and untracked-path semantics across platforms. |

### Shared tasks: lint

| File and task | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `.devtools/tasks/lint.toml`: `devtools:lint-workflows` | 6 | Shared task. consumer CLI/CI | B30 | B: sequence `devtools:lint-workflows` commands. Preserve shell integration tests. |
| `.devtools/tasks/lint.toml`: `devtools:lint-source-files` | 33 | Shared task. consumer CLI/CI | B23 | G4: path selection and generated-file exclusions need one portable code. |

### Shared tasks: pins

| File and task | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `.devtools/tasks/pins.toml`: `devtools:update-pins` | 77 | Shared task. consumer CLI/CI (not selected here) | B27 | G1: pin parsing and Git updates need portable configuration handling. |
| `.devtools/tasks/pins.toml`: `devtools:check-pins` | 100 | Shared task. consumer CLI/CI (not selected here) | B27 | G1: check configured pins portably against Git state. |

### Shared approval tasks

| File and task | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `.devtools/tasks/preapproval.toml`: `devtools:preapprove` | 105 | Shared task. consumer CLI/CI | None | B: preserve approval command sequencing in the real shell. Preserve shell integration tests. |
| `.devtools/tasks/preapproval.toml`: `devtools:revoke-preapproval` | 29 | Shared task. consumer CLI/CI | B06 | G7: grant scope and state handling must preserve authority across platforms. |

### Shared Python language tasks

| File and task | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `.devtools/tasks/python.toml`: `devtools:lint-py-all` | 7 | Shared task. consumer CLI/CI | None | B: invoke the Python lint tools from Bash. Preserve shell integration tests. |
| `.devtools/tasks/python.toml`: `devtools:mutate-py` | 5 | Shared task. consumer CLI/CI | None | B: coordinate Python mutation commands. Preserve shell integration tests. |
| `.devtools/tasks/python.toml`: `devtools:audit` | 5 | Shared task. consumer CLI/CI | None | B: compose tools that audit dependencies with shell glue. Preserve shell integration tests. |

### Shared tasks: security

| File and task | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `.devtools/tasks/security.toml`: `devtools:gitleaks` | 12 | Shared task. consumer CLI/CI | B24 | G1: interpret scanner status and reject empty-history scans portably. |

### Shared tasks: toolchain

| File and task | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `.devtools/tasks/toolchain.toml`: `devtools:check-toolchain` | 10 | Shared task. consumer CLI/CI | B22 | B: sequence `devtools:check-toolchain` commands. Preserve shell integration tests. |

### Shared tasks: vendir

| File and task | source lines of code | Callers | Bats coverage / runtime reference | Decision and reason |
| --- | ---: | --- | --- | --- |
| `.devtools/tasks/vendir.toml`: `devtools:check-vendored` | 15 | Shared task. consumer CLI/CI (not selected here) | B22 | G1: compare tracked and untracked output without shell path splitting. |

## Bats suites and shared helper

`test-hooks` calls every suite under `tests/` recursively. `test-leaks` calls the payload suite. `mise run test` now selects Go and Python suites from the diff. `mise run test-full` runs both and checks generated skills. The preceding baseline predates that selection and measures every suite. Consumer fixture suites test the published payload. For B suites, parser cases move with the code they test. G5 suites keep real Git smoke tests while their parsing and state checks move in process. These timings measure the whole suite once and aren't additive estimates of individual script costs.

### Configuration suites

| ID | Suite | source lines of code | Callers | Runtime seconds | Decision and reason |
| --- | --- | ---: | --- | ---: | --- |
| B01 | `tests/config/starter-vocabulary.bats` | 42 | test-hooks. CI | 1.49 | B: small vocabulary contracts need a shell. |

### Shared tooling suites

| ID | Suite | source lines of code | Callers | Runtime seconds | Decision and reason |
| --- | --- | ---: | --- | ---: | --- |
| B02 | `tests/devtools/lint-commit-msg.bats` | 79 | test-hooks. CI | 0.75 | B: the draft preview keeps its shell boundary. |

### Git environment suite

| ID | Suite | source lines of code | Callers | Runtime seconds | Decision and reason |
| --- | --- | ---: | --- | ---: | --- |
| B03 | `tests/git-environment.bats` | 20 | test-hooks. CI | 0.09 | B: environment isolation depends on Bash. |

### Hook and script suites

| ID | Suite | source lines of code | Callers | Runtime seconds | Decision and reason |
| --- | --- | ---: | --- | ---: | --- |
| B04 | `tests/scripts/clear-commit-agentmsg.bats` | 46 | test-hooks. CI | 0.14 | B: draft cleanup remains a real hook check. |
| B05 | `tests/scripts/codex-fix-prose-state.bats` | 53 | test-hooks. CI | 0.36 | B: prose state keeps a shell-level contract. |
| B06 | `tests/scripts/codex-preapproval.bats` | 130 | test-hooks. CI | 2.02 | B: approval grants still need command-level fixtures. |
| B07 | `tests/scripts/commit-trailers.bats` | 149 | test-hooks. CI | 0.36 | Migrated to Go golden cases in `internal/commitlint`. |
| B08 | `tests/scripts/commitlint.bats` | 141 | test-hooks. CI | 0.41 | Conventional Commits cases now run under Go tests. |
| B09 | `tests/scripts/skill-tokens-backends.bats` | 77 | test-hooks. CI | 0.37 | P1: assert token backend text and manifest fixtures directly. |

### Workflow suites: reports

| ID | Suite | source lines of code | Callers | Runtime seconds | Decision and reason |
| --- | --- | ---: | --- | ---: | --- |
| B10 | `tests/skills/analyze-session/scripts/analyze-session.bats` | 202 | test-hooks. CI | 3.07 | P1: test transcript and report processing without shell subprocess fixtures. |

### Workflow suites: publication and commits

| ID | Suite | source lines of code | Callers | Runtime seconds | Decision and reason |
| --- | --- | ---: | --- | ---: | --- |
| B11 | `tests/skills/commit/scripts/commit-workflow.bats` | 334 | test-hooks. CI | 8.55 | G5: exercise commit parsing and state without subprocesses. |
| B13 | `tests/skills/merge-pr/scripts/merge-workflow.bats` | 588 | test-hooks. CI | 9.48 | G5: test merge transactions through direct state assertions. |
| B14 | `tests/skills/pr/scripts/pr-workflow.bats` | 491 | test-hooks. CI | 21.18 | G5: check publication workflow data within the process. |

### Workflow suites: prose and writers

| ID | Suite | source lines of code | Callers | Runtime seconds | Decision and reason |
| --- | --- | ---: | --- | ---: | --- |
| B12 | `tests/skills/fix-prose/scripts/fix-prose.bats` | 158 | test-hooks. CI | 1.70 | G5: move prose workflow state checks into unit cases. |
| B21 | `tests/skills/writers/permissions.bats` | 84 | test-hooks. CI | 0.13 | B: writer permissions keep an actual shell boundary. |

### Workflow suites: replay and monitoring

| ID | Suite | source lines of code | Callers | Runtime seconds | Decision and reason |
| --- | --- | ---: | --- | ---: | --- |
| B15 | `tests/skills/rebase/scripts/rebase-workflow.bats` | 102 | test-hooks. CI | 0.71 | G5: check rebase state with library calls. |
| B16 | `tests/skills/resolve-rebase-conflicts/scripts/resolve-workflow.bats` | 85 | test-hooks. CI | 0.60 | G5: assert conflict classification and resolution directly. |
| B20 | `tests/skills/watch-pr/scripts/watch-checks.bats` | 110 | test-hooks. CI | 0.50 | G5: assert watcher state directly within the process. |

### Workflow suites: shared library

| ID | Suite | source lines of code | Callers | Runtime seconds | Decision and reason |
| --- | --- | ---: | --- | ---: | --- |
| B17 | `tests/skills/source/lib/gates.bats` | 111 | test-hooks. CI | 1.91 | G5: test gate-cache state in the Go process. |
| B18 | `tests/skills/source/lib/lib.bats` | 404 | test-hooks. CI | 2.69 | G5: replace shared-library parsing fixtures with unit tests. |
| B19 | `tests/skills/source/lib/masked-status.bats` | 193 | test-hooks. CI | 0.74 | G5: assert status propagation without repeated shell launches. |

### Consumer task suites: adoption and tooling

| ID | Suite | source lines of code | Callers | Runtime seconds | Decision and reason |
| --- | --- | ---: | --- | ---: | --- |
| B22 | `tests/tasks/adoption.bats` | 108 | test-hooks. CI. consumer fixtures | 2.73 | B: consumer adoption needs a thin command contract. |
| B23 | `tests/tasks/exclusions.bats` | 262 | test-hooks. CI. consumer fixtures | 7.00 | B: source exclusions keep shell integration coverage. |
| B24 | `tests/tasks/gitleaks.bats` | 90 | test-hooks. CI. consumer fixtures | 1.47 | B: scanner invocation warrants small Bash fixtures. |
| B25 | `tests/tasks/gomodscan.bats` | 84 | test-hooks. CI. consumer fixtures | 0.45 | B: module scanning still needs command sequencing checks. |
| B26 | `tests/tasks/languages.bats` | 115 | test-hooks. CI. consumer fixtures | 1.25 | B: language task adapters require shell-level assertions. |

### Consumer task suites: configuration and output

| ID | Suite | source lines of code | Callers | Runtime seconds | Decision and reason |
| --- | --- | ---: | --- | ---: | --- |
| B27 | `tests/tasks/pins.bats` | 127 | test-hooks. CI. consumer fixtures | 0.59 | B: pin tasks keep a consumer invocation contract. |
| B28 | `tests/tasks/python-templates.bats` | 40 | test-hooks. CI. consumer fixtures | 0.30 | B: the Python templates keep minimal shell fixtures. |
| B29 | `tests/tasks/vendor.bats` | 59 | test-hooks. CI. consumer fixtures | 0.17 | B: vendor tasks need a small command-level suite. |
| B30 | `tests/tasks/workflows.bats` | 66 | test-hooks. CI. consumer fixtures | 0.33 | B: workflow lint commands keep Bash integration cases. |

### Repository tool suites

| ID | Suite | source lines of code | Callers | Runtime seconds | Decision and reason |
| --- | --- | ---: | --- | ---: | --- |
| B31 | `tests/tools/check-versions.bats` | 143 | test-hooks. CI | 0.72 | P1: assert release literals and manifest fixtures directly. |
| B32 | `tests/tools/generate-skills.bats` | 136 | test-hooks. CI | 0.89 | P1: test generated skill text and manifests without subprocess fixtures. |
| B33 | `tests/tools/masked-status.bats` | 225 | test-hooks. CI | 0.85 | B: command failures still need shell status checks. |
| B34 | `tests/tools/release-dispatch.bats` | 67 | test-hooks. CI | 0.25 | B: dispatch wrapping uses a small real-shell suite. |
| B35 | `tests/tools/release-readiness.bats` | 94 | test-hooks. CI | 0.57 | G6: use temporary Git repositories around portable release checks. |
| B36 | `tests/tools/release-verify.bats` | 74 | test-hooks. CI | 0.56 | G6: check portable tag verification in temporary repositories. |

### CI workflow suites

| ID | Suite | source lines of code | Callers | Runtime seconds | Decision and reason |
| --- | --- | ---: | --- | ---: | --- |
| B37 | `tests/workflows/check-preset-refs.bats` | 157 | test-hooks. CI | 1.18 | B: preset references keep a shell task contract. |
| B38 | `tests/workflows/release.bats` | 96 | test-hooks. CI | 0.32 | B: release commands still require a thin Bash check. |
| B39 | `tests/workflows/sarif.bats` | 23 | test-hooks. CI | 0.10 | B: SARIF filtering keeps a command-level fixture. |
| B40 | `tests/workflows/security.bats` | 57 | test-hooks. CI | 0.15 | B: security task composition needs shell integration. |
| B41 | `tests/workflows/skill-scanner.bats` | 88 | test-hooks. CI | 0.12 | B: scanner task invocation keeps a minimal Bash check. |

### Payload suite and shared helper

| ID | Suite | source lines of code | Callers | Runtime seconds | Decision and reason |
| --- | --- | ---: | --- | ---: | --- |
| B42 | `.devtools/scripts/check-leaks.bats` | 1199 | test-leaks. consumer | 12.25 | G5: move scanner cases in process and keep Git-object integration tests. |
| N/A | `tests/test_helper.bash` | 24 | Suite setup through source | Included in caller suite times | B: the Git environment isolation and gate stubs require a real shell. |

## Migration order

| Order | Targets | Dependency, portability, and runtime reason | Validation or adoption condition |
| ---: | --- | --- | --- |
| 1 | Port `.devtools/scripts/check-leaks.sh`, `leak-skip.sh` and pre-push installation into the #174 repository tooling. | First portability-sensitive scanner and consumer hook boundary. B42 takes 12.25 seconds. | Replace regular expression/pipeline and Git-object cases with Go tables. Use a small real-push suite. |
| 2 | Port PR description validation and readers, the shared `lib.sh`, and commit/PR/rebase/merge preflight checks together with #174’s review and repository libraries. | Other explicitly prioritized portability risks in #203. Start with publication validation in B14 (21.18 seconds), then review and shared state in B18 (2.69 seconds) and B17 (1.91 seconds). | Preserve configuration authority, quota statuses, stamps, and refusal contracts before reducing the large workflow suites. |
| 3 | Migrate publication, commit, merge, and rebase transactions, then the checks watcher and hook guards into #174. | Within that dependency order, B13 merge transactions (9.48 seconds) and B11 commits (8.55 seconds) precede the smaller B15 rebase and B20 watcher suites. | Split tests of pure parsing and state from the small set requiring real repositories, hooks, and subprocesses. Don't delete old behavior cases before replacements pass. |
| 4 | Port suppression checks, conflict/path handling, consumer pin editing and generated-file selection. Move report generators and token/manifest processing to Python. | Replace GNU/BSD-sensitive parsing and Bash array work first. B10 transcript reporting takes 3.07 seconds. | Require full branch coverage for the Python processing. |
| 5 | Reduce remaining shell adapters and consumer task fixtures to command sequencing and real-shell contracts. Replace the copied bootstrap installer with thin glue. | Start with B23 source exclusions (7.00 seconds) and B22 adoption fixtures (2.73 seconds). | Re-measure three full test runs and the Bats share under comparable load after each group. Land the gate at 200 lines of source code only once all Bash sources meet it. |
| 6 | File consumer adoption follow-ups for o11y, tailnet-policy, space-geese-project, [development hub](https://github.com/tbhb-dev/devhub), and vale-ai-tells. | After releasing the replacements. | Keep #203 open for ports, the final gate, and before/after evidence. |
