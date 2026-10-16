That script checks the review stamp and records what the staging area contains before it commits, then reads the commit back and compares. prek stashes and restores the worktree around the pre-commit hooks, so a failed run can leave a staged path unstaged, and the retry would commit part of the group without reporting it. It prints every path it expected and didn't find. It reads against the first parent, so a merge commit and a root commit compare correctly. Its output is the check, so nothing needs a `git show --stat` after it.

It also compares the commit's tree with the tree the reviewer read. When a pre-commit hook fixes a file in place, prek fails the commit outright: stage the fix by name and review again, since the staged tree moved. The tree comparison catches the rarer hook, one that rewrites the index and lets the commit through, and then the script says to compare and amend.

The script pins `commit.cleanup=whitespace` for you. At the default of `strip`, git drops every body line opening with a number sign, and it does so after the review has hashed the file. The bytes the reviewer cleared would then stop matching the bytes git records.

Only `--amend` and `--confirmed` pass through. `--amend` is the form `fix-pr` routes here for, and it needs a review run with `--amend` too. The script refuses every other flag, because a check on the command line sees `bash commit.sh` and none of the flags underneath it.

Then rebase onto the base from step 1, without asking. Skip this when step 1 found nothing to rebase onto.

```text
git -c rebase.updateRefs=false -c rebase.autoSquash=false rebase --autostash <base>
```

`rebase.updateRefs` quietly moves other local branches that point into the replayed range. `rebase.autoSquash` collapses any `fixup!` commit, each of which passed its own review.

`--autostash` scopes the save to the rebase itself. Never a bare `git stash pop`: worktrees share one stash stack, so a bare pop can take another session's entry.

Where that rebase stops on a conflict, hand it to the `rebase` skill rather than resolving it here. That skill classifies each conflicted path before resolving it, then checks what the replay did to each commit.

Report the resulting commit, then the groups still waiting from step 2, if any.

## Preconditions

Preflight checks the toolchain this skill assumes, and `references/PRECONDITIONS.md` beside this file lists it. When a precondition is missing, tell the operator rather than improvising a substitute. The workflow commits only the repository holding the session. A sibling checkout has no stamp of its own, so open a session there to commit it.
