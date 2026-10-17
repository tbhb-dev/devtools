## Step 8: route the rest

Follow the answer from step 6.

`Open only` ends here. Say the pull request is open and hand back.

`Open and watch` invokes `watch-pr` with the number, then reports what it found. Stop there even when something failed.

`Open, watch, and fix` invokes `watch-pr`, then `fix-pr` on failure, then `watch-pr` again to confirm. Repeat until the checks pass, then stop before merging. Bound the loop. After three rounds on the same check, stop, then report what each attempt changed.

`Open, fix, and merge` does the same, then invokes `merge-pr` with the number once the checks pass. That skill drafts the squash message and puts it through its own review. It also asks the operator to confirm before merging, which gives the merge a confirmation of its own.

Where the description drifts as remediation commits arrive, redraft it, review it again, and re-run `create-pr.sh` to update the published copy.
