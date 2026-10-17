## Step 1: confirm the pull request is mergeable

Preflight printed the state, the checks, and the review decision. Merging needs an open pull request that nobody marked as a draft, with every check green.

For each check, preflight judges its latest run. A workflow that started twice on one head and cancelled the first run leaves those cancelled jobs behind, and preflight leaves them out and says how many it left out. Without a later run, a cancelled run still counts as failing.

A failing check goes to `fix-pr`, and a running one goes to `watch-pr`. Neither belongs here, and the merge script refuses both anyway.

A `mergeStateStatus` of `BEHIND` or `DIRTY` is the third case. The branch needs the base underneath it before anything merges, so run the `rebase` skill and push the result. Don't reach for GitHub's own update button: it merges the base into the branch, which leaves a merge commit this workflow never wrote a message for.

For `REVIEW_REQUIRED` or `CHANGES_REQUESTED`, preflight exits 75 with `WAITING_REVIEW`. Resolve the required approvals, then rerun. Review checks before invoking the reviewer. Merge checks again before the merge state.
