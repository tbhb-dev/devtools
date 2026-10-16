## The inversion, once

During a rebase git checks out the base and replays your commits onto it. Your work is the side arriving, so git calls it `--theirs`, and the base you are moving onto becomes `--ours`. Every intuition about which word means which runs backwards here, and a wrong pick resolves cleanly and stays wrong.

This skill avoids both words. The sides are:

- `base-side` is the commit the branch replays onto. git calls it `--ours`, and the index calls it stage 2.
- `replayed-side` is the commit git is replaying right now. git calls it `--theirs`, and the index calls it stage 3.

Because the scripts read the index stages rather than the marker labels, the naming is correct under whatever marker style the repository sets.

## Marker shape

With `merge.conflictStyle` set to `diff3` or `zdiff3`, a conflict has three sections rather than two:

<!-- rumdl-disable merge-conflict -->

```text
<<<<<<< HEAD
the base-side content
||||||| <ancestor label>
what both sides started from
=======
the replayed-side content
>>>>>>> <commit> (<subject>)
```

<!-- rumdl-enable merge-conflict -->

That `|||||||` section is the common ancestor, and it's the most useful part of the block: it says what each side changed rather than only what each side now contains. The default `merge` style omits it. Handle both, and never assume the two-way form.

A bare `=======` line isn't a marker on its own. Because it underlines a setext heading in ordinary Markdown, the scripts here match only `<<<<<<<`, `|||||||`, and `>>>>>>>`.
