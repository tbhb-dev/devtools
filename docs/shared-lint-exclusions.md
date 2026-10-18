# Shared lint exclusions

Consumers can include `.devtools/tasks/lint.toml` in mise's `task_config.includes` and call `devtools:lint-shell`, `devtools:lint-shell-fmt`, and `devtools:lint-markdown-wrap` from their local lint task. These tasks require shellcheck, shfmt, and guard-markdown on PATH, respectively.

The consumer owns tool installation, including guard-markdown.

These tasks select tracked and untracked files through Git and skip an empty selection. They apply the repository's ignore rules to untracked files. By default, they skip `vendor/`, `apm_modules/`, and exact dependency `deployed_files` paths from `apm.lock.yaml`, without excluding descendants of its directory entries. This lets the gates check first-party skills and rules under `.claude/`, `.agents/`, and `.codex/`. Without a lockfile, only the vendor and generated-path exclusions apply. Paths containing spaces remain individual arguments. Markdown accepts guard-markdown arguments, including `mise run devtools:lint-markdown-wrap -- --fix`.

Set `DEVTOOLS_GENERATED_PATHS` to a newline-separated list of repository-relative generated directories or files to extend those defaults. Directory entries may end with a slash. Entries use Git `pathspec` patterns. A directory excludes its descendants as well. Put this example in the consumer's mise configuration:

```toml
[env]
DEVTOOLS_GENERATED_PATHS = """
internal/generated
web/generated
CHANGELOG.md
"""
```

Because generated paths have no universal names, the shared tasks exclude only the paths the consumer declares. Keep real source outside those entries so violations still fail the gates.

The Go architecture starter at `.devtools/config/go/.go-arch-lint.yml` excludes the same third-party and deployed directories, along with its existing `testdata` and `tools` entries. Copy that starter into the consumer's configuration and add generated directories to its `exclude` list. The shared architecture task also applies Git ignore rules and `DEVTOOLS_GENERATED_PATHS` through a temporary configuration, preserving the consumer's existing exclusions. It requires uv, which runs the Python helper with its pinned YAML parser. Direct go-arch-lint calls still require tool-installed directories under `exclude`.

The `devtools:lint-prose` template uses the same Git selection through `devtools:lint-source-files`. Include `.devtools/tasks/lint.toml` and keep Vale scope in `DEVTOOLS_PROSE_GLOB`. The default is `*`. Explicit path arguments narrow the Git selection. Ignored files stay excluded. The task uses Vale's `ai-tells-agent.tmpl` output template.
