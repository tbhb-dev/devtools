That says which commit stopped, how far through the replay it sits, which paths remain unmerged, and what git staged without asking.

When another workflow started the rebase, preflight reconstructs the durable before/base record from git's active rebase state, so verification still has both ranges after the replay finishes.

Then invoke the `resolve-rebase-conflicts` skill. It classifies each path before anything edits it and settles the mechanical ones itself, which covers most of them in this repository. Don't resolve conflicts here. The classification is what keeps a mechanical case from turning into a decision someone makes by hand. Keep the resolution in this thread, because delegated agents share the filesystem and concurrent conflict edits are unsafe.

Once you have staged every path:
