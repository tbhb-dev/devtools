## Step 1: choose the rebase base

Preflight computed this under `== rebase base ==`. Take its recommendation:

- Local default branch carries everything the remote has, so rebase onto the local branch.
- Local default branch sits behind the remote, so rebase onto `origin/<default>` and skip the stale local copy.
- `recommended base: none` means there is nothing to rebase onto. A repository's first commit is the usual case. The other is a branch whose default branch is missing both locally and on origin. Record that, and step 9 skips its rebase.

Record the base now. Step 9 rebases onto it without asking again.
