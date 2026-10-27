# Adopt the shell command allow list

`vendir` copies `.devtools/shell-allowlist.toml` with `.devtools/**`.

The file is inert until a caller runs the shell analyzer.

Consumers don't need to change their configuration when it arrives.

This change doesn't add gate wiring or automatic file discovery.

To try the analyzer from a devtools checkout, specify each file:

```sh
go run ./cmd/shell-allowlist \
  --allowlist .devtools/shell-allowlist.toml script.sh
```

The command checks only the supplied files. Exit codes are:

- 0: no findings
- 1: findings
- 2: a read or parse error

Later changes cover executable distribution and gate integration.

Function bindings must precede calls within their execution scope.

The analyzer reports calls when it can't establish those bindings.

Quoted command words preserve literal backslashes.

Unsupported escape forms and expanding words produce findings.

Name checks can miss tools that a permitted wrapper invokes.

Don't treat this check as an execution sandbox.
