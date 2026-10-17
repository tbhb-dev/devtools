# Shared lint exclusions

Consumers can include `.devtools/tasks/lint.toml` in mise's `task_config.includes` and call `devtools:lint-shell`, `devtools:lint-shell-fmt`, and `devtools:lint-markdown-wrap` from their local lint task. These tasks require shellcheck, shfmt, and guard-markdown on PATH, respectively.

The consumer owns tool installation, including guard-markdown.

All three tasks select tracked and untracked files through Git and skip an empty selection. They apply the repository's ignore rules to untracked files. Their default exclusions are `vendor/`, `apm_modules/`, `.claude/`, `.agents/`, and `.codex/`. Paths containing spaces remain individual arguments. Markdown accepts guard-markdown arguments, including `mise run devtools:lint-markdown-wrap -- --fix`.

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

The Go architecture starter at `.devtools/config/go/.go-arch-lint.yml` excludes the same third-party and deployed directories, along with its existing `testdata` and `tools` entries. Copy that starter into the consumer's configuration and add generated directories to its `exclude` list. The environment list affects the shared shell and Markdown tasks. Go-arch-lint reads its own configuration.

## Integration handoff

The release integration package owns root task wiring. Replace local shell and Markdown wrap recipes with calls to the shared tasks, preserving any repository-specific tool invocation the repository still needs. This repository currently runs guard-markdown from source, while the shared task uses the installed command. You don't need to change the README or agent instructions in this package.

Run `mise exec -- bats tests/tasks/exclusions.bats` for consumer fixtures covering empty repositories, tracked and untracked violations, default exclusions, and generated directory and file extensions.
