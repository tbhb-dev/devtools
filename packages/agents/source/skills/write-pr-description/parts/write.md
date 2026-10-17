## The layout

Write the file in exactly this order.

```text
---
base: <branch this merges into>
draft: <true or false>
labels: [<one or more from the set above>]
reviewers: []
assignees: []
milestone:
---

# <type>(<scope>)?: <description>

## <first template section>

<prose>
```

Those frontmatter keys and no others. Labels take a flow sequence and an empty one fails the validator, because nobody's filter finds an unlabelled pull request. Leave `milestone:` empty where there is none. `null` and `~` read the same way, so neither specifies a milestone.

The title is a level 1 heading in the Conventional Commits shape. A squash merge turns it into the commit subject on the default branch, so its type has to be one the landing commits use, and it states what the whole branch does rather than what the last commit did.

Then every section the template declares, in the template's order, filled with prose. Replace each instructional comment rather than leaving it in place.

## One question per section

The sections answer different questions, so don't let them repeat each other. Summary says what changes. Why says what problem made it necessary. Verification names the commands you actually ran, in backticks. Risk says what breaks if this is wrong and how to back it out. Related points at issues, or says `None`.

Avoid:

- Restating the diff. A reviewer reads the diff already.
- Counting. No file, line, test, or commit counts.
- Provenance filler. Nothing about requests, sessions, prompts, models, or tools.
- Claims you haven't checked, including verification nobody ran.
- Selling it. Drop `robust`, `comprehensive`, `significantly`, and their neighbors.
- Paths that don't exist. The validator checks each backticked path against the tree and the diff.

## Revising rather than starting over

Where a draft already exists, the context printed it earlier. Keep the sentences that are still accurate. A revision that rewrites untouched prose costs the caller another review round for nothing, and it loses wording an earlier review already cleared.

A finding points at one instance of a fault rather than the fault itself. Fix the instance, then read the rest of the draft for the same fault, because a reviewer that found one instance finds the next one on the next round. That sweep isn't the rewriting this section warns against, since it changes a sentence only where the sentence is wrong.

Where the published description differs from the draft, the branch has moved since the last publish. Bring the draft forward rather than reverting it to what GitHub shows.

## Read the draft back before returning

The validator covers what a script can check. The reviewer sends drafts back for the faults listed below, and the context printed earlier already contains the answer to each, so a round trip spent on one buys nothing.

**Closed enumerations.** A sentence listing the kinds of change on the branch claims to list every one, and a reviewer reads it that way. Walk the changed-file listing against every sentence of that form, and confirm the sentence covers each path or that you left the path out on purpose. Where you can't confirm it, write the sentence open, so it claims only what you checked.

**Scope words.** `only`, `every`, `never`, and `the one place` each make a claim about the files you didn't quote. Open the file and establish the scope before writing one. A setting described as mattering on one CI slot is a claim about every other slot, and the workflow in the diff either bears that out or doesn't.

**The Avoid list earlier in this document.** Read the finished draft against it once. Following a rule while writing and satisfying it in the finished text are different things.
