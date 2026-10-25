## Arguments

Give the file first, then the command that judges it.

```text
COMMIT_AGENTMSG mise run lint-commit-msg
PR_AGENTDESC.md mise run lint-pr-description
docs/design.md mise run lint-draft docs/design.md
```

Only the caller knows which gate applies, and it supplies the command. A commit draft takes the commit-msg scope, and a description takes its own task. Anything else takes `mise run lint-draft`, which calls `agentrepo lint-draft` through the payload shim and runs vale and cspell and rumdl together over one file.

Prefer a task that runs every checker at once. `mise run lint-prose` runs vale alone, so a document clearing it can still fail spelling and structure, and a verdict from that task is narrower than it sounds.

Where the request doesn't specify a file, ask which one.
