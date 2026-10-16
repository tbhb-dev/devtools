That rebuilds `PR_AGENTDESC.md` from what the pull request currently says, reassembling the frontmatter from the properties set on the pull request. It refuses to overwrite an existing draft without `--force`, because a `pr` workflow further up the stack may have one in flight.

Merging is the last moment the description can change, and you write the squash message from it, so a wrong description propagates that error into history.

Read what preflight printed under `== description as published ==` against the commits it collapses. Most merges skip the rest of this step. A description that still fits doesn't need a pass, and a pull request nobody here authored has none of this machinery behind it, so take its description as it stands.
