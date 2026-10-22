The workflow wrapper runs the repository's configured `lint-squash-msg` task with the cspell configuration and word list read from the pull request head. It never reads those files from the operator's checkout.

The gate runs the commit hooks along with the source-range identity context that requires `Assisted-by` when the squash collapses an agent commit:

- vale under the commit scope, which catches AI tells through `ai-tells-commits`
- cspell with the commit dictionary
- commitlint for the Conventional Commits shape
- commit-trailers for trailer order

Order matters here. The review in the next step stamps the exact bytes it read, so a lint fix after it voids the stamp and costs another review round over a comma. Before merging, the script runs this task again and refuses when the task is absent rather than skipping it.
