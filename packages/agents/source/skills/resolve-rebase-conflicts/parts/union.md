An append-only sorted list conflicts on nearly every rebase, and the answer comes out the same every time: every line either side has, sorted, once each. A word list, a spelling dictionary, a vocabulary file. Asking an operator to confirm that means asking them to rubber-stamp arithmetic.

Safety comes from the condition rather than the filename. The script refuses unless all three versions really do sort cleanly under the C collation with no duplicate line, and unless both sides kept every line the ancestor had. Those two conditions are what make a union provably lossless. A file that stops meeting them stops resolving here, which is what keeps a name-based shortcut from applying after the property it assumed stops being true.

After writing, the script re-derives the union from the index stages and compares the two. The result must contain exactly the lines the two sides contain between them. No duplicates, and still sorted. A resolution that dropped a line fails there rather than in a lint run three commits later.

Don't confirm any of this with the operator. Report it in the summary at the end.
