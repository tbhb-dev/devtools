## Step 5: run the validator and the prose gates

```text
mise run lint-pr-description
```

That task runs the mechanical checks, then vale and cspell over the draft. The validator settles the frontmatter structure, the title's form and bounds, section presence and order, empty sections, surviving comments, unclosed fences, dead links, and whether every backticked path exists. It reads the title's type and the paths against origin's copy of the base, as the review does. Each finding gives a line and the fix. Section findings (`section-missing`, `section-order`, `section-empty`) end with `; template: <path>`, naming the working-tree template file the validator read.

Resolve every one through `fix-prose` rather than editing it yourself, because a direct edit here spends the context that skill exists to save. Edited the draft either way? Then step 4 runs again, because the stamp binds bytes rather than intentions.
