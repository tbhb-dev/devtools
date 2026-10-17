# Gitleaks fixture policy

Copy `.devtools/config/gitleaks.toml` to `.gitleaks.toml` at the repository root. The `devtools:gitleaks` task runs from that root, where gitleaks discovers the file automatically. The template extends the built-in rules with `useDefault = true`, which you must keep to preserve secret detection when adding an exception. Exceptions aren't active in the template, but its commented example allows only `tests/fixtures/private-key.pem`, a synthetic private key used to test private-key detection. Remove the comment markers from the example only when that exact fixture exists and contains no real credentials. Replace the example path and justification if your fixture serves a different purpose.

Treat changes to `.gitleaks.toml` as security configuration. Review the fixture contents and explain why you must suppress detection there. Anchor each path expression with `^` and `$`, escape literal dots, and specify the individual file. Don't allow entire test directories, extensions, or secret patterns across the repository: a path exception suppresses every secret rule for the matching file.

Never put real credentials in an allowed fixture.

Run `mise run devtools:gitleaks` after committing the policy and fixtures: the task scans git history, so an uncommitted fixture doesn't exercise it. Confirm that the synthetic fixture passes and that a generated secret outside the allowed path still fails. Keep scanner output redacted. This policy is separate from the private identifier gate's `.leak-allow` file.
