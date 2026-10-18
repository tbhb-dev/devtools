# Reviewer deadlines

`agentrepo review run --timeout 900s -- COMMAND [ARGS...]` runs a reviewer in a separate process group on macOS and Linux. It passes stdin through and waits for command exit and output EOF. On expiry or SIGINT/SIGTERM, it kills the group. It waits for the direct child. The operating system reaps orphaned descendants. Descendants must remain in the process group. Other platforms return a failure without launching the command.

| Status | Meaning |
| --- | --- |
| 0 | Command and output streams completed successfully |
| 1 | Launch, I/O, or reviewer exit failure |
| 124 | Deadline expired |
| 130 | Canceled by SIGINT/SIGTERM |

Reviewer exit failures map to 1, including a command that itself exits 124. The timeout status identifies the runner's deadline. A `PASS` printed before a timeout or a failed exit never permits a review stamp.

The skills read `review_timeout` from machine configuration through `dt_conf`, with `DEVTOOLS_REVIEW_TIMEOUT` taking precedence. The value is seconds and defaults to 900. Repository configuration can't set it. When `agentrepo` is absent from PATH, the skills keep their previous unbounded execution and print a warning to stderr. Install the optional consumer pin described in the [adoption steps](../README.md#consumer-adoption) to enable the deadline. In this repository, `mise run install` installs `agentrepo` alongside `guard-markdown` from source.
