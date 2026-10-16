## Step 1: confirm the pull request is mergeable

Preflight printed the state, the checks, and the review decision. Merging needs an open pull request that nobody marked as a draft, with every check green.

Preflight judges each check by its latest run. A workflow that started twice on one head and cancelled the first run leaves those cancelled jobs behind, and preflight leaves them out and says how many it left out. A cancelled run with no later run still counts as failing.

A failing check goes to `fix-pr`, and a running one goes to `watch-pr`. Neither belongs here, and the merge script refuses both anyway.

A `mergeStateStatus` of `BEHIND` or `DIRTY` is the third case. The branch needs the base underneath it before anything merges, so run the `rebase` skill and push the result. Don't reach for GitHub's own update button: it merges the base into the branch, which leaves a merge commit this workflow never wrote a message for.

Where the repository requires a review decision, respect it. `CHANGES_REQUESTED` means the merge waits, whatever the checks say, and the merge script refuses anything but an approval where the repository requires one.
