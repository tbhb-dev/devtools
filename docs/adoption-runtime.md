# Adopt the runtime payload

This is the runtime part of the adoption guide. The integration package can add it to the shared guide after the consumer gates and starter policies land.

## Bootstrap the runtime

Choose one released tag for every devtools reference. Copy the [vendir manifest](../.devtools/vendir.yml) into the consumer as `vendir.yml` and replace both refs with that tag. It delivers the whole `.devtools` tree and the mise drop-ins. Keep both paths in Git so updates can check their contents against the index.

The consumer needs mise and vendir before its first sync. Declare vendir, uv and APM in its root mise tools, then run `vendir sync` and `mise install`. Add the guard tool in the root tools table, using the same tag as the manifest:

```toml
[tools]
"go:github.com/tbhb-dev/devtools/cmd/guard-markdown" = "v0.11.0"
```

The baseline payload pins Go, which builds the guard. Keep this tool declaration in the root `mise.toml`: the pin tasks read and update that location. Replacing the old manual `go install` step puts the executable on the mise task PATH and makes its version part of the update contract. Run agent sessions under `mise exec --` so their hooks inherit that PATH too.

## Configure draft tasks

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

Keep any other task includes the project needs. Both prose tasks have default bodies in the skill-contract drop-in, so consumers can remove their copied helper bodies. The payload supplies `lint-draft.sh`, `fix_prose_replacements.py` and `apm_audit.py`. A regression check compares these helpers with the publisher's current copies until its root task wiring migrates. Consumers don't need their own copies.

Each consumer defines its prose policy.

Supply `.vale.ini`, `.cspell.jsonc` and the Markdown lint config. Scope Vale to the draft paths and run `vale sync` to install its styles and the published `ai-tells-agent.tmpl` template. Draft lint checks that rules cover the target path before running Vale, cspell, and rumdl. For corrections carrying `replace_with`, the replacement task checks the original span before applying them. It leaves other findings for an author. It needs uv and Python 3.14 or later. uv can provision the interpreter.

## Deploy agent packages

Declare agent dependencies in `apm.yml`. The pin tasks accept the bare `tbhb-dev/devtools#vX.Y.Z` reference and the published `packages/agents/common`, `packages/agents/claude`, `packages/agents/codex` and `packages/agents/agy` forms, including quoted YAML strings. An unrecognized package or a reference without a version fails the update before files change. All declared references and the guard tool must specify the same release.

Run `mise run devtools:apm-sync` for the first deployment and after changing agent dependencies. It clears `.hooks` in Claude and Codex settings, `.apm` in the shared agent hook file and the generated hook manifests, then runs `apm install` and audits the result. Settings outside those keys remain unchanged. Those hook keys belong to APM. Keep custom hooks in package declarations so reconciliation can restore them. A failed install or audit fails the task.

Set `DEVTOOLS_LOCK_PLATFORMS` in the root mise environment to the project's supported platform list. Run `mise lock --platform <list>` and commit both mise lockfiles along with the payload and APM deployment. Updates warn and leave lockfiles alone without that variable.

## Verify adoption

To verify adoption, create a draft at a Vale-scoped path, run `mise run lint-draft <file>`, run `mise run fix-prose-replacements <file>` and review the edits. Then run the draft gate again. Check deployment with `mise run devtools:lint-apm`, pins with `mise run devtools:check-pins` and the installed tools with `mise run devtools:check-toolchain`. Stage the payload before running `mise run devtools:check-vendored`, which compares against the index.

## Update to the next release

For the next release, run `mise run devtools:update-pins <version>`.

It resolves the tag and moves APM, guard, vendir, hook, workflow, and preset pins. It then syncs the payload and refreshes configured lock platforms. After installing the tools and reconciling APM, it checks the new pins. The consumer must select `apm.toml` alongside `pins.toml`. It no longer needs to supply an `apm-sync` task. Review and commit the resulting files. Unpinned Renovate presets remain unpinned and cause a disagreement with versioned sites.

For integration, include the new APM task file where you want its shared names. Replace the publisher's helper bodies with the supplied defaults. Wire `devtools:check-pins`, `devtools:check-vendored`, `devtools:check-toolchain` and `devtools:lint-apm` into the consumer's own combined lint task.

The existing vendir gate is the drift checker. Don't copy another checker into `tools/`.
