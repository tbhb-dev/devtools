## Base merge mode

When a branch must keep its published history, fetch origin and merge `origin/<base>` into it. Resolve conflicts and stage the resolved paths by name before review. Preflight accepts only the current fetched tip of the pull request's base, or the repository default branch when there is no pull request. Other merges, octopus merges, rebases, cherry-picks, and reverts still hold. Don't use `--amend` during a merge.

Review reconstructs conflicted paths from both parents after staging and shows each resolution against each parent. The stamp binds both parents and `MERGE_HEAD`. Gates check the merged tree. Verification checks both parents and the SSH signature. Follow the repository's merge-subject rule and keep all required trailers. Skip the post-commit rebase in this mode.
