## Step 1: choose the base

Preflight computed this under `== rebase base ==`. Take its recommendation.

- Local default branch carries everything the remote has, so rebase onto the local branch.
- Local default branch sits behind the remote, so rebase onto `origin/<default>` and skip the stale local copy.

The `commit` skill's preflight makes the same decision, and both reach the same answer. Where the operator specifies a different base, use theirs and say why it differs.

Nothing to replay means nothing to do. Say so and stop.

## Step 2: settle the working tree

`--autostash` handles a dirty tree, and `start-rebase.sh` passes it. It scopes the save to this rebase and never touches the shared stash stack, which is what makes it the sanctioned form here.

The stashed work is outside the worktree while you resolve a conflict, so nothing you see there reflects it. It can also conflict on the way back once the replay finishes.

Where the work has to persist through a cancelled rebase as well, park it in a throwaway commit first:

```text
git commit -am wip --no-verify
```

That's the one sanctioned `--no-verify` in this repository, and `git reset --soft HEAD~1` afterwards removes it before anything reaches history.

Never a bare `git stash pop` or `git stash apply`. Worktrees share one global stash stack whose indices shift whenever any worktree pushes. A bare pop takes whatever entry is at `stash@{0}` right then.
