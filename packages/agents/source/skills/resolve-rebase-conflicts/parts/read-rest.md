For a `content` conflict, work from what each side was trying to do rather than from the marker block. The classifier already printed both diffs against the common ancestor. Where you need more:

```text
git diff :1:<path> :2:<path>    # what the base-side changed
git diff :1:<path> :3:<path>    # what the replayed-side changed
```

Resolve so that the result keeps both intents. Keeping one side's edit and quietly dropping the other is the failure to watch for, and afterwards it looks identical to a clean resolution. Nothing you can run at this point tells the two apart. For that reason the `rebase` skill range-diffs the whole branch at the end rather than trusting that the replay applied.

One thing in a `token-union` proposal needs the operator: the ordering. Nobody disputes the union of the additions, but where those new tokens belong in the line is a judgement about how the file reads. Print the proposal, ask, apply the answer.

Stage what you resolved, by name:

```text
git add -- path/one path/two
```

Never `git add -A` or `git add .`. During a rebase those sweep in whatever an earlier commit in the replay left behind.
