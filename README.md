# devtools

Shared agent tooling for tbhb repositories. The repo provides Go command-line tools for agent harnesses, plus the shared [APM](https://microsoft.github.io/apm) package of agent primitives.

## APM package

Repositories across tbhb install the shared primitives with the [APM CLI](https://microsoft.github.io/apm/quickstart/):

```bash
apm install tbhb-dev/devtools#v0.19.0
```

A consumer can also select a sub-package, such as `apm install tbhb-dev/devtools/packages/agents/claude#v0.19.0`. The pin tasks support both forms.

The package deploys these primitives from sub-packages under [`packages/agents/`](packages/agents/): `claude` contains the skills below plus the `guard-markdown` hook declaration, `codex` contains the Codex skill variants, and `common` contains the harness-neutral `worktree-wip` instructions.

| Primitive | Type | Purpose |
| --- | --- | --- |
| `commit` | skill | Group changes into one atomic commit, draft the message in `COMMIT_AGENTMSG`, review it, lint it, confirm it, then commit and rebase. |
| `review-commit-message` | skill | Review a drafted message against the staged diff as an independent agent, for what linting can't see. |
| `worktree-wip` | instructions | Stash and work-in-progress rules for repos that run more than one agent worktree session. |
| `guard-markdown` | hook | `PreToolUse` gate on `Write` and `Edit` that refuses Markdown whose paragraphs span more than one line. |

Pinning the bare repo (`apm install tbhb-dev/devtools#v0.19.0`) resolves the root workspace manifest, whose dependencies on the four sub-packages under `packages/agents/` are local paths that a remote consumer's `apm` resolves transitively, filtering each sub-package by its own targets. For a claude-only consumer, the bare pin gives exactly what the `claude` and `common` pins deliver. APM filters the other packages by their declared targets. This repo installs its own package the way a consumer does: `apm install` deploys the primitives into the local harness layout, and CI rejects drift between the `packages/agents/*/.apm/` sources and the deployed copies.

## Checks

A check is one rule enforced everywhere it matters. Because all three callers run the same binary, the rule can't drift between them:

| Caller | Invocation |
| --- | --- |
| Claude `PreToolUse` | `guard-markdown --hook` reads the payload on stdin and denies the `Write` or `Edit` |
| pre-commit | `guard-markdown FILES` reports the offending line ranges and exits nonzero |
| verification stack | the same command from a mise task, alongside the other linters |

[`.pre-commit-hooks.yaml`](.pre-commit-hooks.yaml) publishes the pre-commit leg:

```yaml
repos:
  - repo: https://github.com/tbhb-dev/devtools
    rev: v0.19.0
    hooks:
      - id: guard-markdown
```

Before running consumer pin checks, freeze the preceding hook tag to its full commit SHA and keep a `# frozen: vX.Y.Z` comment, as shown in the pin forms below.

Note that the two legs install separately. prek builds its own copy from `language: golang`. The Claude hook instead resolves `guard-markdown` on PATH, which comes from the managed mise tool declaration in the adoption steps below. A consumer wanting both adds the prek hook and the mise tool.

Pass `--fix` to collapse paragraphs someone already wrapped.

## Go tools

Each tool under [`cmd/`](cmd/) builds as a standalone binary:

- [`agentrepo`](docs/checks-evaluator.md) evaluates saved GitHub checks. This binary owns repository transactions, signing, and delegation as they move into Go.
- `agy-otel hook <event>` silently records Antigravity hook events in a local spool.
- `agenthooks` manages shared agent hooks.
- `agentstore` stores shared agent state.
- `agentcontext` assembles shared agent context.
- `guard-markdown` refuses Markdown whose paragraphs span more than one line.
- [`commit-lint`](docs/commit-lint.md) checks Conventional Commits and attribution trailers.
- [`gh-post`](docs/gh-post.md) scans GitHub content before posting under an allowed identity.
- [`ssh-sign`](docs/agent-signing.md) signs agent commits through a dedicated SSH agent.

Install one directly:

```bash
go install github.com/tbhb-dev/devtools/cmd/agenthooks@latest
```

The next minor release includes `agy-otel`. After that tag exists, pin the recorder through mise:

```toml
[tools]
"go:github.com/tbhb-dev/devtools/cmd/agy-otel" = "v0.15.0"
```

The current v0.14.2 release doesn't contain this command. Hook registration remains an operator task. The recorder stores events under `$XDG_STATE_HOME/agy-otel`, falling back to `~/.local/state/agy-otel`.

Or build everything from a checkout:

```bash
mise run build
```

## Development

[mise](https://mise.jdx.dev) pins the toolchain and runs the workflow: `mise run bootstrap` sets up a fresh clone and builds the commands into the ignored `bin/` directory, `mise run lint` runs the full lint suite, and `mise run test` runs the tests. Run `mise run install` explicitly to put the commands on `PATH` in the mise-managed Go installation's bin directory. On a machine with no mise, the committed shim at `tools/mise-bootstrap` runs in place of `mise` and installs the pinned release on first use. The pins live in [mise.toml](mise.toml) and the drop-ins under `.config/mise/conf.d/`, locked by digest in the committed `mise.lock` files. See [AGENTS.md](AGENTS.md) for the agent-facing contributor guide.

After creating a linked worktree, inspect its configuration and run `mise trust`, then run `mise run devtools:worktree-setup`. The setup task refuses an untrusted configuration. Once trusted, it prepares the worktree-local Vale styles with `vale sync` and the Python environment with `uv sync`. Skill setup checks stop with these commands when the configuration is untrusted or the ignored Vale styles are absent.

The test task includes Go, Python, hook Bats, and leak Bats suites. Run `mise run test-hooks` or `mise run test-leaks` for the shell suites. CI runs them in `test-bats (ubuntu-24.04)` and `test-bats (macos-15)`. See the [task and CI routing guide](AGENTS.md#tasks) for the check names `fix-pr` recognizes.

### Review and delegated runs

Commit messages, PR descriptions, and squash messages pass script-owned independent reviews. Repositories can supply [review policy](AGENTS.md#repository-review-policy) in `.agents/review-instructions.md` on origin’s trusted default branch. Policy changes need ordinary review before later branches can use them, and policy can't waive credential checks or select a reviewer.

Delegated writers have scoped tools for gathering context, editing, and validation. A delegated write gets confirmation through the machine’s `delegation_check` when no explicit confirmation or session grant applies. See [confirmation authority](AGENTS.md#pre-approving-the-confirmations) and [the writer scanner exception](AGENTS.md#skill-sources). The exception covers one scanner rule in the two writer manifests. Other scanning remains enabled.

PR, merge, and watch readers use GraphQL so REST core quota exhaustion doesn't produce a false identity mismatch. Quota failures stop with exit 76 and reset guidance. The scripts never retry writes automatically. See [API quota behavior](AGENTS.md#github-api-quota-in-pull-request-skills) for read costs and remaining REST use.

## Consumer adoption

### Bootstrap the runtime

These instructions describe the current main branch. Choose a released version that includes `gh-post` before adopting its posting skills.

Choose that release for every devtools reference once it's available. Copy the [vendir manifest](.devtools/vendir.yml) into the consumer as `vendir.yml` and replace both refs with that tag. It delivers the whole `.devtools` tree and the mise drop-ins. Keep both paths in Git so updates can check their contents against the index. Existing consumers syncing only `.devtools/tasks` must expand the manifest to `.devtools/**` with `newRootPath: .devtools`.

The consumer needs mise and vendir before its first sync. Declare vendir, uv and APM in its root mise tools, then run `vendir sync` and `mise install`. Add the guard, commit-message, and posting tools in the root tools table, using the same tag as the manifest:

```toml
[tools]
"go:github.com/tbhb-dev/devtools/cmd/guard-markdown" = "<devtools-release>"
"go:github.com/tbhb-dev/devtools/cmd/commit-lint" = "<devtools-release>"
"go:github.com/tbhb-dev/devtools/cmd/gh-post" = "<devtools-release>"
```

Configure the posting identity allowlist and private-list policy before posting. The [GitHub posting guide](docs/gh-post.md) covers installation, configuration keys, supported operations, exit codes, and the local audit log.

The optional `agentrepo` declaration enables [reviewer deadlines](docs/reviewer-timeout.md) once pinned to a release containing `review run`.

Add it to the root tools table. Replace `<release-containing-review-run>` with the same released devtools version used by the other pins, choosing a release that includes `review run`:

```toml
"go:github.com/tbhb-dev/devtools/cmd/agentrepo" = "<release-containing-review-run>"
```

`devtools:update-pins` rewrites this declaration when present. `devtools:check-pins` checks it when present and recommends it when absent without failing.

The baseline payload pins Go, which builds both required commands. Keep these tool declarations in the root `mise.toml`: the pin tasks read and update that location. Replacing old manual `go install` steps puts the executables on the mise task PATH and makes their versions part of the update contract. Run agent sessions under `mise exec --` so their hooks inherit that PATH too.

### Configure draft tasks

Select the runtime tasks in the consumer's root config:

```toml
[task_config]
includes = [
  ".devtools/tasks/apm.toml",
  ".devtools/tasks/pins.toml",
  ".devtools/tasks/toolchain.toml",
  ".devtools/tasks/vendir.toml",
]

[tasks.lint-draft]
extends = "devtools:lint-draft"

[tasks.fix-prose-replacements]
extends = "devtools:fix-prose-replacements"
```

Keep any other task includes the project needs. Both prose tasks have default bodies in the skill-contract drop-in, so consumers can remove their copied helper bodies. The payload supplies `lint-draft.sh`, `fix_prose_replacements.py` and `apm_audit.py`. Consumers don't need their own copies.

Each consumer defines its prose policy.

Supply `.vale.ini`, `.cspell.jsonc` and the Markdown lint config. Scope Vale to the draft paths and run `vale sync` to install its styles and the published `ai-tells-agent.tmpl` template. Draft lint checks that rules cover the target path before running Vale, cspell, and rumdl. For corrections carrying `replace_with`, the replacement task checks the original span before applying them. It leaves other findings for an author. It needs uv and Python 3.14 or later. uv can provision the interpreter.

### Deploy agent packages

Declare agent dependencies in `apm.yml`. The pin tasks accept the bare `tbhb-dev/devtools#vX.Y.Z` reference and the published `packages/agents/common`, `packages/agents/claude`, `packages/agents/codex` and `packages/agents/agy` forms, including quoted YAML strings. An unrecognized package or a reference without a version fails the update before files change. All declared references and the guard tool must specify the same release.

Run `mise run devtools:apm-sync` for the first deployment and after changing agent dependencies. It clears `.hooks` in Claude and Codex settings, `.apm` in the shared agent hook file and the generated hook manifests, then runs `apm install` and audits the result. Settings outside those keys remain unchanged. Those hook keys belong to APM. Keep custom hooks in package declarations so reconciliation can restore them. A failed install or audit fails the task.

The commit and rebase skills use rebase to bring branches current. Consumers that bring pull request branches current by merging can keep Git's generated subjects: `Merge branch '<b>'[ of <url>][ into <b>]`, `Merge remote-tracking branch '<r>'[ into <b>]`, and `Merge commit '<sha>'[ into <b>]`. The `shape` rule accepts these forms only when `MERGE_HEAD` exists for the pending commit or the outgoing commit has more than one parent. Other merge subjects and all non-merge subjects still require Conventional Commits syntax. Length, body, trailer, and leak checks still apply. Repository rules still forbid merge commits on main.

Set `DEVTOOLS_LOCK_PLATFORMS` in the root mise environment to the project's supported platform list. Run `mise lock --platform <list>` and commit both mise lockfiles along with the payload and APM deployment. Updates warn and leave lockfiles alone without that variable.

### Pin forms

Keep tag references and commit references aligned. The pin tasks read these forms:

| Site | Form |
| --- | --- |
| `apm.yml` | `tbhb-dev/devtools#vX.Y.Z` or a supported agent sub-package with the same suffix |
| Root `mise.toml` | Guard and commit-message tool versions `vX.Y.Z` in `[tools]` |
| `vendir.yml` | Both devtools Git refs use `vX.Y.Z` |
| `.pre-commit-config.yaml` | Full commit SHA with `# frozen: vX.Y.Z` |
| `.github/workflows/*.yml` or `*.yaml` | devtools `uses` calls use a full commit SHA with `# vX.Y.Z` |
| `.github/renovate.json5` | devtools preset ref uses `#vX.Y.Z`, or a commit SHA with a `// vX.Y.Z` comment |

### Verify adoption

To verify adoption, create a draft at a Vale-scoped path, run `mise run lint-draft <file>`, run `mise run fix-prose-replacements <file>` and review the edits. Then run the draft gate again. Check deployment with `mise run devtools:lint-apm`, pins with `mise run devtools:check-pins` and the installed tools with `mise run devtools:check-toolchain`. Stage the payload before running `mise run devtools:check-vendored`, which syncs again and compares against the index, including new files hidden by personal ignore rules.

### Update to the next release

For the next release, run `mise run devtools:update-pins <version>`. It rewrites tracked prose naming the previous pinned release and lists each changed file and line. Mentions of other versions are unchanged for a manual edit.

Upgrading to v0.13.0 requires the preceding `commit-lint` declaration. Add it before running `devtools:update-pins`. The update and pin-check tasks now treat it as a required site alongside `guard-markdown`.

It resolves the tag and moves APM, tool, vendir, hook, workflow, and preset pins. It then syncs the payload and refreshes configured lock platforms. After installing the tools and reconciling APM, it checks the new pins. The consumer must select `apm.toml` alongside `pins.toml`. It no longer needs to supply an `apm-sync` task. Review and commit the resulting files. Unpinned Renovate presets remain unpinned and cause a disagreement with versioned sites.

Wire `devtools:check-pins`, `devtools:check-vendored`, `devtools:check-toolchain` and `devtools:lint-apm` into the consumer's combined lint task. Keep local policy outside the vendored directories.

## Language tasks

Select other task files in the same `task_config.includes` array as the adoption tasks, omitting language files you don't use.

| Task file under `.devtools/tasks/` | Consumer use |
| --- | --- |
| `go.toml` | Go lint, test, build, coverage, vulnerability, and vendor tasks |
| `rust.toml` | Rust tasks. See [Rust adoption](docs/rust-adoption.md) |
| `python.toml` | Python lint, test, coverage, mutation, and fuzz tasks |
| `lint.toml` | Shared shell, Markdown, spelling, config, and workflow gates |
| `security.toml` | Gitleaks scanning |
| `leaks.toml` | Private identifier scans and prek installation |
| `preapproval.toml` | Session grants, checks, and revocation. See [consumer scopes](docs/consumer-preapproval.md) |

Run `mise run devtools:lint-go` or `mise run devtools:test-go`. Omit either language file when unused. Local wrappers such as `test-go` can run `mise run devtools:test-go`. In devtools, these wrappers keep task names without a namespace. The shared conf.d pins supply the Go executables. Install Python tools in the consumer's uv dependency groups and lockfile. Go tasks using `go tool` also need the corresponding tool dependencies in the consumer's `go.mod`.

Set overrides in `[env]`: `DEVTOOLS_GO_PACKAGES` defaults to `./...`, `DEVTOOLS_GO_BIN_PACKAGES` to `./cmd/...`, and `DEVTOOLS_GO_MODULES_MODE` to `readonly` (use `vendor` for a vendored module). Build metadata derives `DEVTOOLS_GO_MODULE` from `go list -m`. `DEVTOOLS_GO_BUILDMETA` defaults to that module's `internal/buildmeta` package, with string variables `Version`, `Commit`, and `Date`. Go ignores those linker assignments when the package is absent. Python source roots default to `src`: set `DEVTOOLS_PYTHON_DIRS` for `pylint`, complexipy, and vulture, `DEVTOOLS_PYTHON_SOURCE_DIRS` for bandit and coverage, and `DEVTOOLS_PYTHON_PROPERTY_DIRS` for fuzzing (default `tests/property`). Directory lists use shell word splitting and glob expansion, so paths must contain no whitespace. Ruff, pyrefly, `pytest`, and cosmic-ray read the consumer's own config for their scope and policy. Both complexipy and vulture also read local policy.

Copy the needed templates from [`.devtools/config`](.devtools/config) to the project root, then adapt these fields. The Python templates use `.template` suffixes so tools don't discover them inside the vendored payload. Remove that suffix when copying them to the root. These are starting points. Syncing updates the templates without overwriting local policy.

| File | Consumer adaptations |
| --- | --- |
| `go/.golangci.yml` → `.golangci.yml` | Replace `example.com/project` in the prefix for `gci`, the module-path for gofumpt, and the local-prefixes for `goimports`. Set the license and copyright template for `goheader` to your authors. Review dependency bans and thresholds. |
| `go/.go-arch-lint.yml` → `.go-arch-lint.yml` | Replace components' `in` paths and `deps` edges with your layers. Adjust `exclude` for your tooling and fixtures. |
| `go/.testcoverage.yml` → `.testcoverage.yml` | Replace `local-prefix` with your module path. Choose thresholds and excluded entry points. Keep `profile: coverage.out` for these tasks. |
| `go/.gremlins.yaml` → `.gremlins.yaml` | Adjust `unleash.exclude-files` for generated or third-party code. The default excludes vendor. |
| `python/ruff.toml.template` → `ruff.toml` | Adjust `exclude` and the test/script patterns under `lint.per-file-ignores`. Choose your rules and documentation string convention. Existing `[tool.ruff]` settings in `pyproject.toml` can stay there instead. |
| `python/pyrefly.toml.template` → `pyrefly.toml` | Set `project-includes` and `project-excludes`. Keep the diagnostic catalog aligned with your pinned pyrefly version. Existing `[tool.pyrefly]` settings can stay in `pyproject.toml`. |
| `.devtools/scripts/go-ldflags.sh` | No edits: set the module or metadata package through env if the defaults differ from your layout. `tools/go-ldflags.sh` in this repo forwards to it. |

The payload's `lint.toml` supplies `devtools:lint-shell`, `devtools:lint-workflows`, and `devtools:lint-curl`. Call the shell and curl tasks from the consumer's combined lint task. The shell task enables shellcheck's `check-extra-masked-returns` rule (SC2312), and the workflow task enables it in workflow shell blocks. SC2312 flags command substitutions and other command uses that hide a failure status. Capture output separately and check the command's status before testing its output. SC2155 remains enabled for declarations that mask failures. For a harmless case, put `# shellcheck disable=SC2312 # diagnostic formatting cannot change the failed verdict` immediately before the affected command and replace the reason with the actual justification.

`devtools:lint-curl` rejects curl calls without `--fail`, `-f` (including short-option bundles), or `--fail-with-body`, including bodies piped, captured, or written to a file. It accepts `-w '%{http_code}'` or `--write-out '%{http_code}'` when it recognizes a comparison of the result. An intentional error-body check can use `# lint-curl: allow the test asserts on the HTTP error body` immediately before the curl command's starting line. Replace the reason with the actual justification. The reason is mandatory, and the comment exempts only the first curl call on that next line. The shared baseline pins uv to run the included Python checker.

Never accept pre-existing findings through a baseline. Fix them or document a harmless local exception with a reason. Don't exclude owned source paths or turn off these rules to accept existing findings. Commands hidden behind `eval`, variables, wrappers, or quoted remote shell strings still need review.

Use the [shared lint exclusions](docs/shared-lint-exclusions.md) when wiring shell and Markdown wrap gates. Default exclusions cover third-party and deployed agent trees. Declare generated paths with `DEVTOOLS_GENERATED_PATHS`. `devtools:lint-workflows` skips repositories without workflow files. `devtools:vendor-check` regenerates Go dependencies and catches tracked and untracked drift, including files hidden by personal ignore rules. It can change the worktree. Review its output.

### Dependency scan gates

Enable the published `gomodscan` hook in a Go consumer's `.pre-commit-config.yaml`. Add `id: gomodscan` to the existing devtools repository entry, keeping its full commit SHA and `# frozen: vX.Y.Z` release comment. `devtools:update-pins` already updates that pin. Install the pre-commit stage with `prek install`.

The hook runs `mise run devtools:gomodscan` only for staged paths matching `^(go\.(mod|sum)|vendor/)` and passes no filenames. Keep `vendor/` out of any global hook exclusion so this gate checks vendored dependency changes. The consumer needs mise, the shared Go tasks, and the gomodscan tool dependency in `go.mod`. Findings block the commit. Execution failures, including unreachable scanner services in a sandbox, also block it and report that the scan couldn't run. Restore service access and retry before committing.

Add both scans to the consumer's `check-all` task so a full local check includes dependency concerns and reachable vulnerabilities:

```toml
[tasks.check-all]
run = [
  "mise run lint",
  "mise run test",
  "mise run devtools:gomodscan",
  "mise run devtools:vuln",
]
```

### Starter policy

Copy the [starter vocabularies](.devtools/config/starter-vocabulary.md) and configure cspell and Vale to read the local copies. Extend them with project terms. The publisher's root dictionaries belong to devtools and aren't consumer defaults.

Copy `.devtools/config/gitleaks.toml` to `.gitleaks.toml` and keep its built-in rules enabled. Follow the [fixture policy](docs/gitleaks.md) for narrowly justified exceptions. These exceptions are separate from the private identifier gate below.

### Consumer workflows

Use the [reusable workflow guide](docs/consumer-workflows.md) for caller jobs, permissions, required tasks, and scanner behavior. Pin workflow calls to a full release commit SHA with a `# vX.Y.Z` comment so the pin tasks can update them. Keep caller job IDs `lint` and `test` stable. The caller supplies test jobs and the reachable-vulnerability gate. Raw govulncheck findings remain available even when code scanning receives only reachable findings.

## Releases

[cocogitto](https://github.com/cocogitto/cocogitto) cuts `vX.Y.Z` tags from the Conventional Commit history. One tag serves both consumer paths, with APM installs pinning `tbhb-dev/devtools#vX.Y.Z` and Go installs pinning `@vX.Y.Z`.

## Python

[`packages/devtools`](packages/devtools) holds the Python side, which is deliberately small for now. Its gates run through `mise run lint-py-all` and `mise run cover-py`, which enforce ruff at its full ruleset, pyrefly's strict preset, and 100% branch coverage. `uv sync` provisions everything from `uv.lock`.

## License

Apache-2.0. See [LICENSE](LICENSE).

## Private identifier gate

Sync `.devtools/scripts/` alongside `.devtools/tasks/` in your consumer's `vendir.yml` (or sync `.devtools/` whole). Include `.devtools/tasks/leaks.toml` in mise's task configuration and add `mise run devtools:lint-leaks` to your lint task. This complements gitleaks with private network addresses, `*.ts.net` host names other than `example.ts.net` and its subdomains, password-manager references, and private-key headers.

Findings report the location and rule without printing matched text. Sensitive locations become opaque identifiers.

Add the `leaks-staged`, `leaks-commit-msg`, `leaks-pre-push`, and `commit-msg-pre-push` hook IDs from this repository to prek. Run `mise run devtools:prek-install` followed by `mise run devtools:install-pre-push-hook` after syncing, and include both steps in your bootstrap task. The latter installs a wrapper before prek's pre-push hook so the scans receive Git's ref lines. The pre-push hook IDs verify the wrapper installation. The wrapper scans the exact outgoing objects and commit messages before prek consumes the ref lines. In CI, fetch full history and run `mise run --quiet devtools:lint-leaks -- --range "$BASE..$HEAD"` for the pull request's base and head commits.

For review worktrees, run `DEVTOOLS_LEAK_HOOK_CHECK=warn mise run devtools:check-leak-hook` (or export the variable before `mise run lint`) to report a missing or mismatched pre-push wrapper as a warning with exit 0. This also applies to `devtools:check-pre-push-hook`. The default check still fails. CI continues to skip the local hook check. The variable doesn't change hook installation or leak scans.

Keep private literals in `mise.local.toml`, which Git ignores:

```toml
[env]
PROJECT_PRIVATE_STRINGS = "private-account-name, private-directory-name"
```

Every exported variable ending in `_PRIVATE_STRINGS` supplies comma-separated, case-insensitive fixed strings. The gate trims whitespace around entries and skips entries shorter than four characters with a warning. Commas and newlines inside a literal are unsupported. Run through mise so it loads the local environment. CI checks publishable patterns without these private values. Never commit the local file.

Add extra case-insensitive extended regular expressions to `.leak-patterns`, one per line. The gate ignores blank lines and lines starting with `#`. For legitimate fixtures, document anchored path expressions in `.leak-allow`, with a comment explaining each exception. These exceptions apply to pattern findings only: the gate checks private literals even in allowed paths. Treat changes to both files as security configuration requiring review. For working-tree scans only, `.leak-skip` skips matching paths entirely. This excludes their contents and private literals from scanning. Use case-sensitive anchored extended regular expressions against repository-relative paths, one per line (such as `^vendor/`). Blank lines and `#` comments have no effect here either. Invalid or unanchored expressions fail the scan. Lint doesn't detect private values already present in a skipped tree. Staged, range, pre-push and explicit-file scans ignore this file and still check private literals. Treat skip changes as security configuration requiring review. The gate scans working files by default, index blobs compared against the commit base with `--staged`, explicit file arguments for commit messages, and complete new Git objects with `--range` or pre-push. Encoded or split values are outside its scope.

`--staged` compares private literal counts by path and list entry with `HEAD`, with `HEAD~1` for an amend, or with the empty tree for a root commit. It maps renamed files to their base paths before counting and reports new occurrences at index line numbers, so line shifts and unstaged edits don't affect the comparison. This mode reads `.leak-allow` and `.leak-patterns` from Git objects in the base tree, while every other mode reads those files from the checkout. Exported `*_PRIVATE_STRINGS` variables supply the private literals in every mode.

The commit workflow pins `DEVTOOLS_LEAK_BASE` and sets `DEVTOOLS_LEAK_MODE=staged` for its direct scan, lint and pre-commit hook, without caching scoped lint as a full-tree pass. Bare `devtools:lint-leaks`, standalone `mise run lint` and CI range scans still scan their full scope. When CI loads the private strings list, its full scan catches literals already in the base that the local comparison allows. The owner must approve the final head and acknowledge this reduction in local coverage.
