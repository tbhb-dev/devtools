# Adopt the shell command allow list

`vendir` copies `.devtools/shell-allowlist.toml` with `.devtools/**`.

The file is inert until a caller runs the shell analyzer.

Consumers don't need to change their configuration when it arrives.

This change doesn't add gate wiring or automatic file discovery.

The library parses shell scripts and reports unlisted commands.

A stacked follow-up adds the command and its tests.

Later changes cover executable distribution and gate integration.

Function bindings must precede calls within their execution scope.

The analyzer reports calls when it can't establish those bindings.

Quoted command words preserve literal backslashes.

Unsupported escape forms and expanding words produce findings.

Name checks can miss tools that a permitted wrapper invokes.

Don't treat this check as an execution sandbox.
