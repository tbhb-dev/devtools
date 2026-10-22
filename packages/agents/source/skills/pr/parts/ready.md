## Step 1: confirm the branch is ready

Preflight answered each of these, so read rather than re-run:

- The branch has commits the base doesn't. Nothing to open otherwise.
- The branch isn't the default branch.
- Uncommitted changes stay out of the pull request. Where preflight listed some, say so and let the operator decide before going on.
- A pull request may already exist. Where one is open, publishing updates it rather than opening a second.
- The branch has a current base. Use `gh pr update-branch` when the base merges cleanly. When it conflicts, fetch origin, merge `origin/<base>`, resolve conflicts, and stage the resolved paths by name. Finish through `commit` or `agent-commit` base merge mode before publishing. This preserves published history without a rebase or force push. When the operator permits rewriting history, the `rebase` skill remains available.

Preflight also settled the draft on disk. It removed a stale `PR_AGENTDESC.md` where no open pull request matched it, then scaffolded a fresh one from the template. That scaffold has the frontmatter keys, a placeholder title, and every section the template declares, so nothing downstream reproduces the template's shape from memory. It fails the validator until something fills it, deliberately.
