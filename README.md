# devtools

Shared agent tooling for tbhb repositories. The repo provides Go command-line tools for agent harnesses, plus the shared [APM](https://microsoft.github.io/apm) package of agent primitives.

## APM package

Repositories across tbhb install the shared primitives with the [APM CLI](https://microsoft.github.io/apm/quickstart/):

```bash
apm install tbhb-dev/devtools/packages/agents/claude#v0.9.0
apm install tbhb-dev/devtools/packages/agents/common#v0.9.0
```

The package deploys these primitives from sub-packages under [`packages/agents/`](packages/agents/): `claude` carries the skills below plus the `guard-markdown` hook declaration, and `common` carries the harness-neutral `worktree-wip` instructions.

| Primitive | Type | Purpose |
| --- | --- | --- |
| `commit` | skill | Group changes into one atomic commit, draft the message in `COMMIT_AGENTMSG`, review it, lint it, confirm it, then commit and rebase. |
| `review-commit-message` | skill | Review a drafted message against the staged diff as an independent agent, for what linting can't see. |
| `worktree-wip` | instructions | Stash and work-in-progress rules for repos that run more than one agent worktree session. |
| `guard-markdown` | hook | `PreToolUse` gate on `Write` and `Edit` that refuses Markdown whose paragraphs span more than one line. |

Pinning the bare repo (`apm install tbhb-dev/devtools#v0.9.0`) resolves the root workspace manifest, whose dependencies on the four sub-packages under `packages/agents/` are local paths. A remote consumer's `apm` resolves those transitively, with each sub-package gated by its own targets, so the bare pin gives a claude-only consumer exactly what the `claude` and `common` pins deliver while the codex and agy scaffolds skip on target intersection. This repo installs its own package the way a consumer does: `apm install` deploys the primitives into the local harness layout, and CI rejects drift between the `packages/agents/*/.apm/` sources and the deployed copies.

## Checks

A check is one rule enforced everywhere it matters. The same binary answers to all three callers, so the rule can't drift between them:

| Caller | Invocation |
| --- | --- |
| Claude `PreToolUse` | `guard-markdown --hook` reads the payload on stdin and denies the `Write` or `Edit` |
| pre-commit | `guard-markdown FILES` reports the offending line ranges and exits nonzero |
| verification stack | the same command from a mise task, alongside the other linters |

[`.pre-commit-hooks.yaml`](.pre-commit-hooks.yaml) publishes the pre-commit leg:

```yaml
repos:
  - repo: https://github.com/tbhb-dev/devtools
    rev: v0.9.0
    hooks:
      - id: guard-markdown
```

Note that the two legs install separately. prek builds its own copy from `language: golang`. The Claude hook instead resolves `guard-markdown` on PATH, which comes from `go install`. A consumer wanting both adds the hook and runs the install.

Pass `--fix` to collapse paragraphs someone already wrapped.

## Go tools

Each tool under [`cmd/`](cmd/) builds as a standalone binary:

- `agenthooks` manages shared agent hooks.
- `agentstore` stores shared agent state.
- `agentcontext` assembles shared agent context.
- `guard-markdown` refuses Markdown whose paragraphs span more than one line.

Install one directly:

```bash
go install github.com/tbhb-dev/devtools/cmd/agenthooks@latest
```

Or build everything from a checkout:

```bash
mise run build
```

## Development

[mise](https://mise.jdx.dev) pins the toolchain and drives the workflow: `mise run bootstrap` sets up a fresh clone, `mise run lint` runs the full lint suite, `mise run test` runs the tests, and `mise run build` compiles the binaries into `bin/`. On a machine with no mise, the committed shim at `tools/mise-bootstrap` runs in place of `mise` and installs the pinned release on first use. The pins live in [mise.toml](mise.toml) and the drop-ins under `.config/mise/conf.d/`, locked by digest in the committed `mise.lock` files. See [AGENTS.md](AGENTS.md) for the agent-facing contributor guide.

## Releases

[cocogitto](https://github.com/cocogitto/cocogitto) cuts `vX.Y.Z` tags from the Conventional Commit history. One tag serves both consumer paths, with APM installs pinning `tbhb-dev/devtools#vX.Y.Z` and Go installs pinning `@vX.Y.Z`.

## Python

[`packages/devtools`](packages/devtools) holds the Python side, kept deliberately small for now. Its gates run through `mise run lint-py-all` and `mise run cover-py`, which enforce ruff at its full ruleset, pyrefly's strict preset, and 100% branch coverage. `uv sync` provisions everything from `uv.lock`.

## License

Apache-2.0. See [LICENSE](LICENSE).
