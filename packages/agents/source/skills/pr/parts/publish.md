That script is the only thing here that publishes, and it gates itself first:

- runs the validator over the draft again
- checks the review stamp against the draft's bytes, `HEAD`, the resolved base, and the merge base, and takes its own copy of the stamped bytes to publish from
- resolves labels, the milestone, and every issue reference against the API, and refuses cleanly on an unknown one rather than leaving a published pull request missing what it should contain
- resolves the confirmation, then checks the stamp again
- checks that `gh` uses the machine's configured `gh_login` for its signed-in account before any write, and says so in its output where the machine configures none

The push is a fast-forward of what origin holds, read from origin itself. A rebase after the last push requires a rewrite. The plain run refuses it. `--rewrite` replaces only the exact commit origin holds, leasing on it. That flag needs the operator's word, and it still refuses where `HEAD` is behind origin or where origin holds a commit this checkout never fetched. Where the operator would rather not rewrite, merge the base into the branch instead. `--dry-run` runs every check and says what the push would do. It warns when the real run would meet a non-fast-forward. It confirms and publishes nothing.

After the push it checks that `HEAD` is still the reviewed commit and that origin holds it. Then it opens the pull request, or brings the open one up to date: title, body, base, labels, reviewers, assignees, milestone, and draft state all converge on the draft, except an existing owner-review label stays in place. Drafts must omit the label named by `owner_review_label`, which defaults to `owner-review`. Only the owner-approval route applies it after approval at the current head, green checks, and a mergeable state. A second run updates rather than opening another.

Use `--dry-run` to check without writing and `--rerun-gates` to force fresh gates. The script can skip checks while the cache key is unchanged: the clean committed tree, command, gate inputs, and tool versions must match. Description checks also bind the draft. It prints each check it skips. Leak scans and the scripted review always run.

Use `--check-description` to run the description validator and prose lint along with labels and property checks. This skips scans, review, lint, tests, and publication. Publication runs description checks, leak scans, lint, the scripted review, and tests in that order.
