That prints a briefing and writes a skeleton, but it doesn't convert the description into a message, because no rewriting rule produces one. The commit that merges here is the one a whole branch leaves behind, so writing it means reading every commit on the branch and working out what a reader years from now still needs.

Your briefing carries the description as published, every commit message the squash collapses in full, the diffstat, and the trailers on those commits. Read it before writing a word. Commit bodies matter most. Each one already argued for itself once, and this is the last place that argument still appears. Run the command on its own and read the whole thing. Piping it through `head` or `tail` keeps the trailing skeleton and drops those bodies, which nothing else in this workflow prints, and the script caps nothing on purpose, so a cap you add is one nobody downstream can see you added.

Its skeleton goes to `SQUASH_AGENTMSG` at the repository root, a file a gitignore entry keeps out of history. Subject and footer come filled in, and the body is empty until you write it.

### Subject

The script sets `<pull request title> (#<number>)`. Keep the reference, because `squash-merge.sh` reads it back to confirm the draft belongs to this pull request. Fix the rest where the title described one commit rather than the branch. A squash leaves one commit behind. Its subject has to describe what the whole stack did.

### Body

The description was Markdown, written for a reviewer with the diff open beside it. This is plain text, and whoever reads it three years from now has neither.

Take from the description:

- Summary and Why, which state the reason the branch exists
- Risk, but only where it describes a rollback the reader wouldn't guess

Leave behind:

- Verification, entirely. It reported what a reviewer needed at the time, and the moment the suite changes it describes a run nobody can repeat.
- Risk that only says the change could be wrong.

Then read the result back against the commits. Trust the commits over it. Authors publish descriptions early while branches keep growing, and the commits record what landed rather than what someone expected to land. Where a commit body gives a reason the description never stated, that reason belongs in the message. Otherwise it goes at the merge, with the commit that held it.

Avoid:

- Restating the diff. A reader who wants the what reads the diff.
- Walking the stack. A branch of eight commits becomes one account rather than eight paragraphs.
- Counting. No file, line, test, or commit counts.
- Provenance filler. Nothing about requests, review rounds, sessions, prompts, models, or tools.
- Markdown, meaning fenced blocks, headings, emphasis, links, or tables. Backticks around a literal identifier are fine.

Hard wrap the body at 72 characters and the trailers at 100.

### Footer

The script fills this in. The trailers come from the commits themselves, ordered with attribution first and sign-off last. `Closes` lines come from the description's Related section, and only for a number GitHub reports as an open issue. A pull request, a closed issue, or a number GitHub wouldn't describe gets a `# Not closed:` line printed under the skeleton instead, outside the message.

Read those lines, then check the `Closes` lines that remain against the branch. An open issue listed under Related isn't always one this branch closes, and a closing keyword acts on merge without asking.
