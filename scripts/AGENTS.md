# Agent instructions

Every script here is a published interface. `.pre-commit-hooks.yaml` names each one as an `entry:`, and consumer repositories reference them through `repo: https://github.com/tbhb-dev/devtools` plus a `rev:`, which resolves the entry path against this repository's root. A path in this directory belongs to the package's public surface the same way an exported function does. Rename one and every consumer breaks at their next `rev:` bump, in their commit hook rather than here. Treat a change to a filename or a calling convention as a breaking change that needs a major version and a coordinated update, not as tidying. The directory contains the remaining shell hooks because they arrived with the fold-in of `tbhb/pre-commit-hooks`, whose hook IDs this repository preserves unchanged so a consumer edits only the `repo:` and `rev:` lines.

`vale-commit-msg.sh` and `cspell-commit-msg.sh` take the commit-message buffer path as their single argument and run on the `commit-msg` stage. They need the consumer to have vale or cspell installed along with a configuration defining the commit-message scope. `clear-commit-agentmsg.sh` runs on `post-commit` instead and takes no arguments. A consumer installing it needs that stage installed too, through `prek install -t post-commit`.

The published `commitlint` and `commit-trailers` hook IDs run the Go command `commit-lint` with rule selectors. Their former shell implementations and Bats suites moved to `internal/commitlint`, whose golden fixtures preserve their diagnostics. See [the linter configuration](../docs/commit-lint.md).

Tests for the remaining shell hooks live in [`../tests/scripts/`](../tests/scripts/), and `mise run test-hooks` runs them. `clear-commit-agentmsg.sh` has a suite. The vale and cspell adapters delegate to external linters.

`mise run lint-script-hygiene` covers `scripts/*.sh` alongside the skill scripts, so the hardening block and the `gitr` wrapper rules apply here too. Read the repository-root [`AGENTS.md`](../AGENTS.md) for those conventions, and [`../tools/AGENTS.md`](../tools/AGENTS.md) for the internal tooling this directory is deliberately not part of.
