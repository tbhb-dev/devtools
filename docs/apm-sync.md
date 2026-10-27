# APM hook reconciliation

Run `mise run apm-sync` in this repository or `mise run devtools:apm-sync` in a consumer. Each task clears obsolete hooks before installing APM primitives, then audits the deployment.

The helper interface remains `.devtools/scripts/apm-sync.sh`.

The helper resolves the mise-selected `agentrepo` before searching PATH. It probes `agentrepo apm-sync --help` before changing settings. Releases without that capability use the frozen shell fallback during N-2 compatibility. A failed Go invocation never triggers that fallback.

`agentrepo apm-sync` clears these keys:

- `hooks` from Claude settings
- `apm` from agent hooks
- `hooks` from Codex hooks

Sibling settings survive. The command removes documents left empty and legacy APM hook sidecars before `apm install`. Missing documents stay absent until installation needs them. Output from installation passes through to the caller, with the same exit status. Each task retains its audit interface and stops on failure.

Cleanup isn't a rollback transaction: an installation failure can leave settings cleared. A later cleanup failure can leave earlier cleanup applied.
