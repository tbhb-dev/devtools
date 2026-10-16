The script snapshots the draft, `HEAD`, the base the draft names, and the merge base, then starts a fresh, read-only reviewer as a headless process of the harness's own CLI, with the `review-pr-description` skill as its instructions.

It resolves the base to `origin/<base>` where origin has a copy, the same commit preflight and the writer compared against, so a stale local branch never adds merged commits to what the reviewer reads. The reviewer watched neither the work nor the drafting, and the script reads its verdict itself.

| Exit | Meaning |
| --- | --- |
| 0 | Passed and stamped |
| 2 | Changes required: the output lists the findings |
| 4 | The reviewer failed or replied without a readable verdict |
| 5 | The draft, `HEAD`, or the base moved while the review ran |

On findings, send them verbatim to `write-pr-description` in its arguments, then run the review again. Bound it at three rounds, counting only the rounds a finding caused. A round the branch caused doesn't count against the bound: a remediation commit or a rebase changes what the description has to describe. Say which kind each round was as you go, so the count is accurate. Past three, stop, report the count with the findings that keep coming back, and let the operator decide whether to go on.

This step is mandatory, and `create-pr.sh` enforces it.

A clean verdict writes a stamp binding the draft's exact bytes, `HEAD`, the resolved base ref, that ref's commit, and the merge base. Findings remove any earlier stamp. Editing the draft, a new commit, or a fetch that moves the base voids it, and the publish refuses until the review runs again.

`DEVTOOLS_REVIEW_CMD`, `DEVTOOLS_REVIEW_MODEL`, and `DEVTOOLS_REVIEW_EFFORT`, or the same keys in `~/.config/devtools/<repository>.conf`, replace the default reviewer. No repository file can.
