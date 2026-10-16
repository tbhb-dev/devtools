Don't type the `git rebase` yourself. The script pins off `rebase.updateRefs`, an operator preference that quietly moves other local branches pointing into the replayed range. It pins off `rebase.autoSquash` as well, which would otherwise collapse a `fixup!` commit that requires its own review. Recording the pre-rebase tip is the third thing it does. Without that record, step 5 has nothing to compare against.

Exit `0` means the replay finished, so go to step 5. A `1` means it stopped, so go to step 4.

Read the output for a line about rerere replaying a recorded resolution. That's git staging a resolution from an earlier rebase without stopping, and nothing has read it since. Treat every such path as unverified.
