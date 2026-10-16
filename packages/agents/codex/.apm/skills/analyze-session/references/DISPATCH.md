# Dispatching agents from Codex

Every step that hands work to other agents uses `spawn_agent` with a short task name and `fork_turns: "none"`. Each agent then starts fresh instead of inheriting this conversation. Fill in the placeholders in the message each reference file gives, and pass that text as the message.

## Locating a session

One fresh agent. The answer is a path, and finding it takes little reasoning.

## Verifying figures

Start three fresh agents with the same message. Batch them when fewer than three slots are available.

Verifiers are read-only. When reproducing a claim requires writes, give each verifier its own explicit temporary directory or worktree and prohibit edits in the caller's checkout. Delegated agents share the caller's filesystem, so conversation isolation alone doesn't isolate files.

## Recommending fixes

Start five fresh agents, one per area, and batch them when fewer than five slots are available.
