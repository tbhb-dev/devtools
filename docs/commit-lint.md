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

Lengths count characters. Bounds must be nonnegative. For header length, the upper bound must cover the lower bound, and types must contain at least one entry. Invalid TOML, invalid environment integers, or invalid bounds cause the command to report a configuration error. Root keys remain stable so future identity settings can use an `[identities]` table. Identity restrictions are outside this command's current rules.

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

This change doesn't add identity settings.
