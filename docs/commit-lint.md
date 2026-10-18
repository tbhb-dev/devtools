# Commit-message lint

`commit-lint [--rules all|shape|trailers] [FILE]` reads a message file or stdin. The default runs both rule groups. Install with `go install github.com/tbhb-dev/devtools/cmd/commit-lint@latest`, or run `go run ./cmd/commit-lint` during development.

The published prek IDs `commitlint` and `commit-trailers` remain available. Both use `language: golang`, which builds the command into prek's managed environment. They select `shape` and `trailers`, respectively. Consumers keep their existing IDs and update the release pin. Direct callers of the removed shell scripts must switch to the command. This repository runs Go from the checkout, so `mise run lint-commit-msg` previews the code you commit through the same local hooks.

## Configuration

Every rule group reads settings from an optional `.commit-lint.toml` file at the repository root. Discovery searches upward from the working directory and stops at the nearest Git root (including a worktree `.git` file).

Without a file, the built-in defaults apply. Empty environment values use the configured value. Nonempty `COMMITLINT_*` values override the file.

| Tom's Obvious, Minimal Language (TOML) key | Default | Environment override |
| --- | --- | --- |
| `types` | `["feat", "fix", "docs", "style", "refactor", "perf", "test", "build", "ci", "chore", "revert"]` | `COMMITLINT_TYPES`, whitespace-separated |
| `header_min_length` | `10` | `COMMITLINT_HEADER_MIN_LENGTH` |
| `header_max_length` | `80` | `COMMITLINT_HEADER_MAX_LENGTH` |
| `body_max_line_length` | `72` | `COMMITLINT_BODY_MAX_LINE_LENGTH` |
| `footer_max_line_length` | `100` | `COMMITLINT_FOOTER_MAX_LINE_LENGTH` |
| `require_signed_off_by` | `true` | `COMMITLINT_REQUIRE_SIGNED_OFF_BY`, integer `1` turns on. Other integers turn off |

Lengths count characters. Bounds must be nonnegative. For header length, the upper bound must cover the lower bound, and types must contain at least one entry. Invalid TOML, invalid environment integers, or invalid bounds cause the command to report a configuration error.

The configuration comes from the current checkout, just as the old environment overrides did. A branch can tighten or loosen these settings, so changes to this policy file need ordinary repository review. This file doesn't provide a trusted-default-branch policy boundary.

## Compatibility

### Subject and body checks

The `shape` group preserves Conventional Commits syntax, known types, header bounds, a blank line after the subject, body and footer length caps, and required sign-off. It strips `#` comments and scissors blocks beginning with `#` or `;`. It also trims trailing whitespace and collapses blank runs before reporting line numbers.

The final block of lines in trailer format and folded continuations uses the footer cap.

### Trailer checks

The trailers group preserves the raw-line checks: kernel-style `Assisted-by` values, assistant attribution before the first sign-off, and rejection of known LLM names in `Co-authored-by`. It retains raw line numbers and recognizes a sign-off prefix even with an empty value. Shape requires a nonempty sign-off value. Raw trailer checks include lines after scissors, matching the former script.

### Diagnostics

The [legacy fixtures](../internal/commitlint/testdata/legacy.json) record the exact output and status from both shell implementations for every former Bats case before their removal. Rule violations keep the `commitlint:` and `commit-trailers:` diagnostic prefixes and exit status `1`. Success exits `0`. Running both groups can report the missing sign-off twice, matching the two former hooks.

### Deliberate changes

Both rule groups now stop requiring sign-off when `require_signed_off_by = false` (or its environment override). Command, input, or configuration errors consistently exit `2` with a diagnostic. These are deliberate changes. Before, the trailer script always required sign-off, and shell or utility errors determined input/configuration failure output and status.

## Sign-off identities

`Signed-off-by` is the responsible person's DCO attestation. An assistant must use `Assisted-by` for its attribution and must never sign off as a configured agent account. Automation bots remain permitted unless the repository explicitly denies them.

The optional `[identities]` table only adds restrictions. Defaults deny nothing. `agents` lists identities forbidden in `Signed-off-by`. The name is also available for future agent-author attribution checks. `sign_off` is an optional allow list: when nonempty, every sign-off must match an entry. Denials take precedence over the allow list and still apply when `require_signed_off_by` is false. No environment variable overrides these lists.

```toml
[identities]
agents = [{ email = "agent@example.com" }]
sign_off = [{ name = "Maintainer", email = "maintainer@example.com" }]
```

Entries require a nonempty `email` pattern. Matching email ignores case and supports `*` (zero or more characters) and `?` (one character). All other characters, including brackets in GitHub bot addresses, are literal. An optional `name` must also match exactly. Omit the name to deny an email regardless of the display name. With restrictions configured, sign-offs must have the form `Name <email>`. The command checks every raw sign-off line. Findings include its line number and identify the responsible person as the required signer.

These settings only tighten the default rules. Removing or changing a committed restriction can still loosen a repository's policy and requires review. They don't establish a trusted-default-branch boundary.

`commit-lint --check-signoff 'Name <email>'` checks an identity with the same configuration. It returns `0` when permitted, `1` for a policy violation, and `2` for configuration or invocation errors. Install `commit-lint` on PATH for the preview and skill scripts as well as in the hooks' managed installation. During development, run `go install ./cmd/commit-lint` after changing it.

The commit skill reads `signoff_identity` from machine configuration (`~/.config/devtools/<repository>.conf`, then `~/.config/devtools/devtools.conf`), or `DEVTOOLS_SIGNOFF_IDENTITY`. Set the complete value, such as `signoff_identity = Maintainer <maintainer@example.com>`. It doesn't derive the signer from Git's author or committer settings. Missing configuration or a denied identity stops preflight. Squash assembly checks carried sign-offs and reports each one it drops. It preserves permitted human and bot sign-offs.
