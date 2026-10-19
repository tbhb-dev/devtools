## Step 2: group the changes into atomic commits

Read the diff before deciding anything. One commit contains one logical change: the reader can state its purpose in a sentence, and reverting it undoes that purpose and nothing else.

Split when the outstanding work covers more than one of these:

- A behavior change and an unrelated refactor
- A fix and the formatting sweep that came with it
- A pair of features that don't depend on each other
- Production code and unrelated tooling or configuration

A dependency doesn't force a split. Code and the test that covers it belong together, as do a change and the documentation that describes it.

When the work splits, commit the first group through this workflow, then say which groups remain and run the workflow again for each. Never bundle them because bundling is quicker.

When one file mixes two logical changes, stage the relevant hunks with `git add -p`, or say so and let the operator decide.

## Step 3: stage the paths for this commit

List every path explicitly:

```text
git add -- path/one path/two
```

Never `git add -A`, `git add .`, or `git add --all`. They sweep in whatever else is uncommitted, which is how an atomic commit stops being atomic.

Confirm the result with `git diff --cached --stat` and `git status --short`. The staged set matches the group from step 2, and anything left unstaged belongs to a later commit.

## Step 4: draft the message in COMMIT_AGENTMSG

Write the whole message to `COMMIT_AGENTMSG` at the repo root. A gitignore entry keeps it out of history, and a post-commit hook deletes it after the commit succeeds.

### Subject

`<type>(<scope>)?(!)?: <description>`, with the type drawn from the list preflight printed. Imperative mood, present tense, lowercase after the colon, no trailing period, and the whole line inside the bounds preflight reported. Write the description as the instruction the commit carries out: `explain the tools`, not `explains` and not `explained`.

### Body

The body answers why this change exists. The diff already says what changed, and a reader who wants the what reads the diff.

Write:

- The problem, the constraint, or the tradeoff that made this the answer
- Why this approach rather than the obvious alternative
- What breaks without it, or where the need came from
- A decision you're recording so nobody relitigates it later

Avoid:

- Restating the diff. `Adds a helper to foo.go and calls it from bar.go` is the diff, spelled out longer.
- Counting. No file, line, test, function, or commit counts. A count goes stale as soon as other work merges, and it reads as padding.
- Provenance filler. Nothing about requests, review rounds, sessions, prompts, models, or tools. The `Assisted-by` trailer records attribution.
- Claims you haven't checked. Leave out benchmark numbers you didn't measure and a `fixes the flake` for a flake you never reproduced.
- Selling it. Drop `robust`, `comprehensive`, `significantly`, `seamlessly`, and their neighbors.
- Markdown. Leave out fenced blocks, headings, emphasis, links, and tables. Backticks around a literal identifier are fine.

Hard wrap the body at the width preflight reported. Wrap trailers at the footer width.

### Trailers

`Assisted-by` before `Signed-off-by`, matching the format and the sign-off identity preflight printed. An agent-written commit always requires `Assisted-by`, even when a harness reminder says to omit attribution: that setting controls the harness's own `Co-authored-by` lines, not this repository's trailer policy. Never credit a model through `Co-authored-by`.
