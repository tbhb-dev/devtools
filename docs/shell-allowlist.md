# Adopt the shell command allow list

`vendir` copies `.devtools/shell-allowlist.toml` with `.devtools/**`.

The file is inert until a caller runs the shell analyzer.

Consumers don't need to change their configuration when it arrives.

This change doesn't add gate wiring or automatic file discovery.

The library parses shell scripts and reports unlisted commands.

Run `shell-allowlist path/to/script` to check each path supplied as an argument.

The command uses the library and the allow list's `commands` entries.

It reports each command with its file, line, and reason. Findings and input errors return exit status 1. Clean input exits 0 without output.

The analyzer ignores heredoc bodies fed to `cat`, including substitutions. It checks substitutions in other heredocs.

Later changes cover executable distribution and gate integration.

Function bindings must precede calls within their execution scope.

The analyzer reports calls when it can't establish those bindings.

Quoted command words preserve literal backslashes.

Unsupported escape forms and expanding words produce findings.

Name checks can miss tools that a permitted wrapper invokes.

Don't treat this check as an execution sandbox.
