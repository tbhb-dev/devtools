```text
mise run lint-squash-msg
```

The same four hooks a commit must pass, plus the source-range identity context that requires `Assisted-by` when the squash collapses an agent commit:

- vale under the commit scope, which catches AI tells through `ai-tells-commits`
- cspell with the commit dictionary
- commitlint for the Conventional Commits shape
- commit-trailers for trailer order

Order matters here. The review in the next step stamps the exact bytes it read, so a lint fix after it voids the stamp and costs another review round over a comma. The merge script runs this task again before it merges, and refuses when the task is absent rather than skipping it.
