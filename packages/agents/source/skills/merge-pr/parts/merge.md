That script runs every gate again before it touches anything. It checks that the message references this pull request, and that the stamp covers the bytes on disk, the live head, the base branch, and the commit that base points at. Where the configuration specifies `gh_login`, the GitHub CLI has to authenticate as that account. Otherwise, the script says it skipped the check. The pull request has to be open, ready, mergeable with a merge state of `CLEAN`, approved where the repository requires a review, and green by the latest run of every check. GitHub reports whether a pull request can merge as `UNKNOWN` for a few seconds after the base moves, so the second of two merges in a row meets it in most cases. The script waits that out for up to a minute before judging, and refuses `CONFLICTING` at once. Then the linters run, and then the confirmation resolves.

The merge specifies the reviewed head, so a commit pushed while the final checks run makes GitHub refuse the merge rather than squash it in. Then the script removes `SQUASH_AGENTMSG` and `PR_AGENTDESC.md` along with their stamps, and deletes the merged branch only when it belongs to this repository. It leaves a fork's branch untouched.

Cleanup falls to the script that watched the merge succeed, because there's no post-merge hook to run it from.

Report the merged commit. Where this worktree stands on the branch that merged, the script says so rather than deleting the branch out from under the session. The operator decides whether to switch or drop the worktree.

## Preconditions

- `gh` installed and authenticated, and `jq`
- a `mise run lint-squash-msg` task
- a gitignore entry for `SQUASH_AGENTMSG`
- the `review-squash-message` skill deployed alongside this one

Preflight checks each. Where one is missing, tell the operator rather than improvising a substitute.
