# Consumer approval scopes

Consumers can declare extra action names in their mise configuration:

```toml
[env]
DEVTOOLS_PREAPPROVAL_SCOPES = "apply deploy-hosts"
```

Names must match `^[a-z][a-z0-9-]*$`. Separate names with whitespace. The shared names `commit`, `pr`, and `merge` are always available. The system reserves `all`, so consumers can't declare it as an extra scope. Undeclared names and invalid configuration fail validation.

The operator grants an action with `mise run devtools:preapprove apply`. Granting uses Touch ID on supported Macs. An operator on a machine without biometrics can explicitly use `--no-biometrics` before the scopes.

Agents must never create their own grants.

The record is an audit trail, and consumer actions that need an access barrier must enforce that separately.

Consumer checks can run `mise run devtools:check-preapproval apply`. It exits zero when the current session holds that declared action. Otherwise it exits one. Invalid names or a missing session also produce exit code one. The checker accepts exactly one action. It rejects `all`. Consumers must include both the task file and `.devtools/scripts/preapproval.sh` in their devtools payload.

`mise run devtools:revoke-preapproval apply` withdraws that action. A bare revoke removes the entire session record, including consumer actions. Explicit `all` permanently means only `commit`, `pr`, and `merge` for both grant and revoke. A bare grant still means only `commit` and `pr`. Grants are additive and keep the original timestamp when repeated. Records stay in the worktree's Git directory under `preapprovals/<session>`. `CLAUDE_CODE_SESSION_ID` takes precedence over `CODEX_THREAD_ID`. A different session doesn't receive a grant from that record. Removing a name from configuration makes the checker reject it. A bare revoke can still remove the record.
