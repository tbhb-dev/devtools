Say what each path was and how you resolved it. Name anything you took a whole side on, and anything a generator still has to rewrite.

Then return to the `rebase` skill, which continues the replay through `continue-rebase.sh`. That script reads the staged blobs for markers once more before it advances.

Where a stop looks like the branch came off the wrong base entirely, say so instead of grinding through the rest. `git rebase --abort` doesn't cost anything.

## A note on rerere

Where `rerere.enabled` is on, git replays resolutions it recorded during earlier rebases and stages them without stopping. git never reports those paths as conflicts, so nothing here classifies them and nothing has read them since the recording. `rebase-status.sh` lists them under `staged without stopping`. Read any that the stopped commit also touches. A resolution git recorded against different surrounding code can apply cleanly and still mean something else now.
