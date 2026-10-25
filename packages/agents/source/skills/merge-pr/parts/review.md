The script snapshots the draft and the pull request, then starts a fresh, read-only reviewer as a headless process of the harness's own CLI, with the `review-squash-message` skill as its instructions. That reviewer hasn't watched you work. It reads the message against every commit the squash collapses, and its first question is the one you can no longer ask yourself by this point: whether the message left out anything that mattered in those commits. The script reads the verdict itself. On a clean verdict it writes a stamp binding the draft's exact bytes, the pull request, its head, its base branch, and the commit that base pointed at. Findings remove any earlier stamp, and so does editing the draft, a push to the branch, or a move of the base.

This step is mandatory, and the merge script enforces it.

| Exit | Meaning |
| --- | --- |
| 0 | Passed and stamped |
| 75 | Waiting for an approving review GitHub requires |
| 2 | Changes required: the output lists the findings |
| 3 | The reviewer rejected the same draft for this pull request head. Change it or ask the operator |
| 4 | The reviewer failed or replied without a readable verdict |
| 124 | The reviewer timed out without a stamp |
| 5 | The draft or the pull request moved while the review ran |

Fix everything it returns and run it again. Push back only where it's demonstrably wrong about the commits, and say why.

The script records every readable verdict with the draft digest and pull request head. After `CHANGES REQUIRED`, it refuses to send byte-identical text for the same head to another reviewer. The reviewer can read a changed draft.

A finding cites one instance rather than the fault itself. Fix the instance, then read the rest of the message for the same fault.

`review_cmd` chooses the reviewer. It isn't only a label. From Codex, configure a reviewer that the session may start. These repository machines use `agy` in read-only plan mode through this line in `~/.config/devtools/devtools.conf`:

```text
review_cmd = agy -p "$(cat)" --mode plan --add-dir "$DEVTOOLS_REVIEW_INPUTS" --print-timeout 900s
```

`DEVTOOLS_REVIEW_CMD`, `DEVTOOLS_REVIEW_MODEL`, and `DEVTOOLS_REVIEW_EFFORT`, or the same keys in `~/.config/devtools/<repository>.conf`, replace the default reviewer. No repository file can, so a branch never chooses who reviews its own merge.

When the unattended finding bound returns exit 3, run `agentrepo review redesign KIND --reason TEXT` with `KIND` set to `commit`, `pr` or `squash`, then rewrite the draft from scratch. A rejected digest for the same subject refuses review before invoking the reviewer. Each active record permits one redesign. A bound after redesign requires inspection and refuses another start. Legacy records without rejected digests require inspection. The command leaves write authority and every other gate unchanged. A fresh description reset, where supported, remains separate from redesign.
