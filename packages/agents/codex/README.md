# devtools-agents-codex

This package holds harness-optimized agent primitives for Codex. Its skills have the same names as the Claude package's. Claude Code reads only `.claude/skills/` and Codex reads `.agents/skills/`, so the two never share a directory and identical names don't collide. For a skill with a source directory under `packages/agents/source/skills/`, `mise run generate-skills` writes the copy here: edit the source, not this copy.

The skills use Codex-native delegated agents, thread identity, process waiting, and operator interaction. Harness-neutral instructions remain in the sibling `common` package.
