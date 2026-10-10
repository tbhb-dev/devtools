# devtools-agents-agy

This package holds harness-optimized agent primitives for Antigravity, accessed through the `agy` CLI. A primitive belongs here once somebody writes agy-specific tooling for it, rather than in the harness-neutral `common` package.

The package is a deliberate scaffold: its layout, CI, versioning, and gitignores are already correct, so the first primitive needs no further setup work. The root manifest starts deploying this package to its target once real content arrives.
