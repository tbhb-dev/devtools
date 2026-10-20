The script snapshots the draft, the staged tree, and `HEAD`, then starts a fresh, read-only reviewer as a headless process of the harness's own CLI, with the `review-commit-message` skill as its instructions. That reviewer hasn't watched you work. It reads the draft against the staged diff with no memory of what you meant to write, and that independence is deliberate. The script reads the verdict itself.

This step is mandatory, and the commit script enforces it. On a clean verdict the script writes a stamp binding the draft's exact bytes, the staged tree, `HEAD`, and whether the review read an amend. Findings remove any earlier stamp, and so does editing the draft, restaging, or moving `HEAD` afterward.

| Exit | Meaning |
| --- | --- |
| 0 | Passed and stamped |
| 2 | Changes required: the output lists the findings |
| 3 | The reviewer rejected the same draft for this staged tree. Change it or ask the operator |
| 4 | The reviewer failed or replied without a readable verdict |
| 124 | The reviewer timed out without a stamp |
| 5 | The draft, the index, or `HEAD` moved while the review ran |

Fix everything it returns and run it again. Push back only when it's demonstrably wrong about the diff, and say why.

The script records every readable verdict with the draft digest and staged tree. After `CHANGES REQUIRED`, it refuses to send byte-identical text for the same tree to another reviewer. The reviewer can read a changed draft.

`review_cmd` chooses the reviewer. It isn't only a label. From Codex, configure a reviewer that the session may start. These repository machines use `agy` in read-only plan mode through this line in `~/.config/devtools/devtools.conf`:

```text
review_cmd = agy -p "$(cat)" --mode plan --add-dir "$DEVTOOLS_REVIEW_INPUTS" --print-timeout 900s
```

`DEVTOOLS_REVIEW_CMD`, `DEVTOOLS_REVIEW_MODEL`, and `DEVTOOLS_REVIEW_EFFORT`, or the same keys in `~/.config/devtools/<repository>.conf`, replace the default reviewer. No repository file can, so a commit never chooses who reviews the next one.
