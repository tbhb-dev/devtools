That script is the only thing here that publishes, and it gates itself first:

- runs the validator over the draft again
- checks the review stamp against the draft's bytes, `HEAD`, the resolved base, and the merge base, and takes its own copy of the stamped bytes to publish from
- resolves labels, the milestone, and every issue reference against the API, and refuses cleanly on an unknown one rather than leaving a published pull request missing what it should contain
- resolves the confirmation, then checks the stamp again
- checks that `gh` uses the machine's configured `gh_login` for its signed-in account before any write, and says so in its output where the machine configures none

The push is a fast-forward of what origin holds, read from origin itself. A rebase after the last push requires a rewrite. The plain run refuses it. `--rewrite` replaces only the exact commit origin holds, leasing on it. That flag needs the operator's word, and it still refuses where `HEAD` is behind origin or where origin holds a commit this checkout never fetched. Where the operator would rather not rewrite, merge the base into the branch instead. `--dry-run` runs every check and says what the push would do. It warns when the real run would meet a non-fast-forward. It confirms and publishes nothing.

After the push it checks that `HEAD` is still the reviewed commit and that origin holds it. Then it opens the pull request, or brings the open one up to date: title, body, base, labels, reviewers, assignees, milestone, and draft state all converge on the draft. A second run updates rather than opening another.

Use `--dry-run` to check without writing and `--rerun-gates` to force fresh gates. For an hour, the script can skip checks that passed for the same clean committed tree. It prints each check it skips. Leak scans and the scripted review always run.

Use `--check-description` to run the description validator and prose lint along with labels and property checks. This skips scans, review, lint, tests, and publication. Publication runs description checks, leak scans, lint, the scripted review, and tests in that order.

The secret gate selects `devtools:gitleaks -- --mode local --base refs/remotes/origin/<declared-base>`. Unattended creation and updates share this gate. The origin base must match the review stamp. Fetch complete history and that exact origin branch first. The gate rejects local fallbacks. The scan covers captured branch-added history and the full index. It deliberately excludes unrelated refs and historical base findings absent from the index. CI retains all-ref coverage. See [the scan contract](https://github.com/tbhb-dev/devtools/blob/main/docs/gitleaks.md).
