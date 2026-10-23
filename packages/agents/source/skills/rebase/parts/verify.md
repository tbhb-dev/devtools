A rebase that ends without complaint has proved one thing: every commit applied. It has proved nothing else, and what it leaves unproved is where the trouble sits.

**A resolution can lose a hunk.** The commit still applies. Its message still describes the change it no longer makes, and no gate anywhere notices, because a commit that does less than it claims isn't a syntax error. The `range-diff` this script prints is what surfaces it. That view pairs each old commit against its replayed self, marking `=` where the commit came through unchanged and `!` where its content moved. Expect a `!` wherever you resolved a conflict. Treat one anywhere else as a question to answer before moving on.

**A tree can break with no commit at fault.** The base adds a gate, the branch adds a file that gate rejects, and neither side fails on its own. That combination first exists after the rebase.

Run the gate the script names:

```text
mise run check
```

Or `mise run lint` where the repository has no `check`. Either way, run it against the tree as it now stands rather than against any commit in isolation, because the tree is the thing nothing has tested yet.

A failure here is an ordinary fix rather than a rebase problem. Correct it, and commit through the `commit` skill. Where the fix belongs inside a commit the rebase just replayed, say so and let the operator choose between a follow-up commit and an amend.

Where the branch runs long enough for a mid-history break to matter, the thorough form runs the gate at every commit:

```text
git -c rebase.updateRefs=false rebase --exec 'mise run lint' <base>
```

The script also lists any path the repository declares generated through `rebase-resolve=regenerate`. The resolution took one side of those whole, on the understanding that the generator would overwrite it. In the devtools checkout, run `mise run regen-generated`. In any other repository, run that repository's own regeneration steps for generated skill files. Commit what it changes.

## Step 6: report

Say what the base was, how many commits replayed, which paths conflicted, and how you settled each one. Give what the gates reported, and name anything the `range-diff` flagged.

Then hand back. Pushing is the caller's call. Because a rebase rewrites the branch, the push needs `git push --force-with-lease origin HEAD`. A bare `--force` is wrong here, because the lease is what keeps the push from discarding whatever arrived while this session worked. Where a pull request is open, `pr` and `watch-pr` take it from there.

## Where this sits

The `commit` skill rebases onto the base itself as its last step, and for a clean replay that rebase is all the branch needs. Come here when that rebase stops on a conflict, when the branch needs moving without a commit in hand, or when the result needs the verification step this skill runs.

## Preconditions

- a git repository with a resolvable default branch
- the `resolve-rebase-conflicts` skill deployed alongside this one
- a `mise run check` or `mise run lint` task for the verification to run

Preflight checks each and says which are missing.
