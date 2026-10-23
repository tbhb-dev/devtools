A generated file is the clear case. Merging two versions of machine output means nothing, because the generator is about to overwrite both. Take either side and move on. After the rebase, in the devtools checkout, run `mise run regen-generated`. In any other repository, run that repository's own regeneration steps for generated skill files. Commit what it rewrites.

A `delete/modify` conflict is the other case, and it isn't mechanical. One side removed the file and the other edited it. Deciding needs the intent behind the removal. Find it, say what you found, then take a side.
