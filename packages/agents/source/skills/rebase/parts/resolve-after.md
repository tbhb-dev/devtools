That reads the staged blobs for conflict markers before it advances. git checks only that no path remains unmerged, so a file you staged with `<<<<<<<` still in it continues straight into a commit.

Repeat from `rebase-status.sh` at each further stop. Where git calls a commit empty because the change is already in the base, `continue-rebase.sh --skip-empty` drops it after proving the index matches `HEAD`.

You can still give up, and doing so doesn't cost anything:

```text
git rebase --abort
```

Use it when the conflicts say somebody cut this branch from the wrong place. Resolving thirty of them to reach that conclusion helps nobody.
