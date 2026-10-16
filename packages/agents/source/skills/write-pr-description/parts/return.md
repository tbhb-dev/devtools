## What to return

Return this block and stop there. Skip the preamble and the praise, and leave the description itself for the caller to read off disk.

```text
DRAFT: WRITTEN

changed: <what you altered, one line per item, or "first draft">
addressed: <each finding from the arguments and how, or "nothing requested">
```

or, when the branch or the repository makes drafting impossible:

```text
DRAFT: BLOCKED

reason: <what stopped you>
```

Blocked covers a branch with nothing to land, a missing template, and a validator finding you can't resolve without a decision the caller has to make. It doesn't cover a validator finding you simply haven't fixed yet.
