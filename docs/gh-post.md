# Scan GitHub content before posting

`gh-post` scans text before a GitHub write and refuses identities outside a machine allowlist. Earlier versions of edited GitHub content can remain visible, so run the check before the first post and every edit. The command supports GitHub.com repositories.

## Install

Choose a devtools release that includes `gh-post`. Add the tool alongside the other devtools pins in the consumer's root `mise.toml`, using the same release for every pin:

```toml
[tools]
"go:github.com/tbhb-dev/devtools/cmd/gh-post" = "<devtools-release>"
```

Run `mise install`. Or install directly with `go install github.com/tbhb-dev/devtools/cmd/gh-post@<devtools-release>`. Both `devtools:update-pins` and `devtools:check-pins` require this declaration and include it in version comparisons. Add it before updating consumers to the skills that require `gh-post`.

Use `gh-post --help` for the complete list of operations and flags, or `gh-post --check-deps` to look for `git` and `gh` on PATH. These informational flags don't need a checkout, configuration, body input, or network access. The dependency check reports each missing binary with exit 2 and executes neither tool.

## Configure

Configuration belongs on the machine. For a target `owner/project`, lookup checks exported `DEVTOOLS_<KEY>` first, then `${XDG_CONFIG_HOME:-~/.config}/devtools/project.conf`, then `devtools.conf` in that directory. Values are literal, unquoted `key = value` assignments. The target repository selects the file, even when `-R` names a different repository from the current checkout.

| Key | Meaning |
| --- | --- |
| `post_identities` | Comma-separated allowed GitHub logins. Empty or absent refuses every post. |
| `scan_mode` | `all` by default. `secrets-only` checks only secrets and doesn't load private strings. |
| `private_strings` | `optional` by default. `required` needs a nonempty list. `none` declares that posting doesn't need a list. Set this policy in a machine file. |
| `private_strings_ref` | 1Password field reference containing comma-separated private literals. A configured reference requires a nonempty result unless policy is `none`. |
| `private_strings_token_command` | Operator-owned executable that prints a 1Password service-account token, called without arguments. Otherwise authentication uses `OP_SERVICE_ACCOUNT_TOKEN`. |

A public repository that doesn't need a private list can use:

```text
post_identities = example-agent,example-app[bot]
private_strings = none
scan_mode = all
```

Exported `DEVTOOLS_PRIVATE_STRINGS` is a comma-separated list, overriding the reference and `none` policy. It isn't a mode switch. Posting stops if an exported list is empty and a reference or `required` policy requires values. An optional list without configuration permits the built-in checks alone. A configured reference that fails or returns no values stops posting. Provider tokens and output aren't printed. The provider requires service-account authentication and doesn't use an interactive login.

The identity guard reads `gh api user`. When that endpoint fails, it reads the GraphQL viewer login instead, including for App installation credentials in `GH_CONFIG_DIR`. Configure the exact returned login in `post_identities`.

## Post

Pass body text through `--body-file PATH`, `--body-file -`, or stdin when the flag is absent. The command refuses inline `-b` and `--body`. The wrapper constructs the GitHub CLI arguments from an allowlist. It rejects other flags such as automatic body generation or templates. It reads the body once and sends those scanned bytes through stdin. It also scans titles, release tags, and other flag values.

For a PR edit with empty stdin and no `--body-file`, the existing description is unchanged. This lets skills scan metadata edits such as a new base for a dependent PR.

Use an explicit empty body file to clear a description.

```bash
gh-post pr create -R owner/project --head topic --base main \
  --title 'feat: add the example' --body-file description.md
gh-post issue comment 42 -R owner/project --body-file comment.md
gh-post pr review 42 -R owner/project --approve --commit "$head" --body-file review.md
gh-post review edit 42 789 -R owner/project --body-file correction.md
gh-post comment edit 123 -R owner/project --kind issue --body-file correction.md
gh-post comment edit 456 -R owner/project --kind review --body-file correction.md
gh-post release edit v1.0.0 -R owner/project --body-file notes.md
```

Supported operations are `issue create|comment|edit`, `pr create|edit|comment|review|ready|close`, `review edit`, `comment edit`, and `release create|edit`. Issue and PR targets are numeric IDs. Comment edits require `--kind issue` for issue comments or `--kind review` for inline review comments. Both use REST `PATCH` endpoints. Release operations translate body input to GitHub's notes-file flag. Creation requires an explicit title for issues and pull requests. New reviews require exactly one of `--approve`, `--comment`, or `--request-changes`.

New reviews also require `--commit <40-hex-sha>` and a first body line of `Reviewed head: <sha>` matching that commit. You can wrap the SHA in backticks. Before posting, the command reads the pull request's `headRefOid` through `gh` and refuses a different head with exit 1. Re-read the head and review it again after a push. The command sends JSON fields `commit_id`, `event`, and `body` through `POST /repos/{owner}/{repo}/pulls/{pr}/reviews`. The explicit `commit_id` anchors the review to the examined commit even if a push races the write. Existing scan, identity guard, and audit requirements apply.

Before scanning or contacting GitHub, new reviews and issue comments whose first line with text starts with `Refinement:` must have closed fenced code blocks. A fence opens with up to three spaces followed by at least three backticks or tildes. To close the fence, use the same character and at least as many markers. Only spaces may follow its markers. Backtick fence info strings can't contain backticks. Refinement comments must also end with `Round: <n> of 3`, where `<n>` is 1, 2, or 3. Blank lines may follow it. A missing closing line or unclosed fence produces one diagnostic line and exit 1 before the scan or any GitHub call. Other body kinds keep their existing checks.

Use `review edit <pr> <review-id>` to replace a submitted pull request review's body. Both IDs must be positive integers. It sends the scanned body as JSON through `PUT /repos/{owner}/{repo}/pulls/{pull_number}/reviews/{review_id}` with the same identity guard and audit record as other posts. It accepts the shared repository and body-file flags. `comment edit --kind review` edits an inline review comment.

Use `-R owner/project` or `--repo owner/project` to select the target. Otherwise the command resolves the checkout's origin remote. Ambient `GH_REPO` and `GH_HOST` don't redirect it. The built-in checks cover local file URLs, home paths with a name segment, GitHub tokens, private-key headers, and secret-manager references. Placeholders such as `/Users/<name>/` and `/home/<name>/` pass. The command matches private literals without regard to case. Findings print only a category and line number, with line numbers local to each text field.

### Change PR state

Use `pr ready` to mark an open draft ready for review, or `pr close` to close an open PR with a reason comment:

```bash
gh-post pr ready 42 --repo owner/project --head "$head"
gh-post pr close 42 --repo owner/project --head "$head" --comment-file reason.md
gh-post pr close 42 --repo owner/project --head "$head" --comment-file reason.md --dry-run
```

Both require a full 40-character hexadecimal `--head`, compared without regard to case. The initial read checks the PR state and head before a state write. A second read verifies the result before the command prints the PR URL. Neither accepts `--body-file` or reads stdin. Close requires a nonempty reason file, limited to the comment limit of 65,536 characters, and posts its scanned bytes through the same path as `pr comment`. Ready requires a draft. Close accepts an open PR whether it's a draft or ready for review. Both use the existing token and allowed-identity check and can change any PR in the target repository regardless of author. They don't add a credential or permission.

Only these two verbs accept `--dry-run`. It checks input, scan, identity, head and state, prints the proposed change, and doesn't write to GitHub. Close also describes the reason comment it would post.

Before posting a close reason, the command reads all comment pages and skips an identical comment by the current identity. If the comment posts but closing fails, the error reports the comment and the state read back. A later invocation with the same file reuses that comment. A comment write failure stops before closing. The command never retries a write automatically.

GitHub doesn't offer an expected-head condition for these state mutations. A push between the initial check and write can race it. If read-back fails or reports a different head or unexpected state, the command exits nonzero with one diagnostic line. It doesn't retry or undo the write. Inspect GitHub before taking further action.

## Results and audit

| Exit | Meaning |
| --- | --- |
| 0 | Posted and recorded in the audit log, or an informational flag or dry run succeeded. |
| 1 | Refused by a scan, identity, head, PR state, or review first-line rule. Also a read-back mismatch after a state write. |
| 2 | Missing dependency, invalid arguments, configuration or input failure, failed GitHub command, or audit failure. |

Each successful post appends one JSON line to `${XDG_STATE_HOME:-~/.local/state}/devtools/gh-post.log`. It records UTC time, operation, the target URL, and the SHA-256 hash of the body. It never records the body. If the command can't open the audit log, it prevents posting. A failed write or an audit failure after posting needs inspection before retrying, because GitHub may already have accepted the content.

Skills require `gh-post` for content publication. When it's missing they stop with an explicit error. They don't fall back to a direct GitHub write.
