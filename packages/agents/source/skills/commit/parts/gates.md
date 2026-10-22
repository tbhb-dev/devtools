## Step 7: run the commit-msg gates

```text
mise run lint-commit-msg
```

That task mirrors the commit-msg hook:

- vale under the commit scope, which catches AI tells through `ai-tells-commits`
- cspell with the commit dictionary
- commitlint for the Conventional Commits shape
- commit-trailers for trailer order

Step 5 should have left this clean. Where it hasn't, send the findings back to `fix-prose` rather than editing the draft here, because a hand-edit spends the context that skill exists to save.

Edited the draft to make the linters pass? Then step 6 runs again before you commit, because the stamp binds bytes rather than intentions.

Whatever this task reports, `.git/COMMIT_EDITMSG` and its commit-msg hook stay the real gate. A clean run here only predicts that hook's verdict.

The commit gate selects `devtools:gitleaks -- --mode local` for normal and amended commits, including unattended entry points. It requires the origin default symbolic ref and complete history. It scans captured branch-added history plus the full index, covering unchanged files as well as partially staged bytes. Local-only bases have no fallback. This deliberately excludes unrelated refs and historical base findings absent from the index. CI retains all-ref coverage. See [the scan contract](https://github.com/tbhb-dev/devtools/blob/main/docs/gitleaks.md).
