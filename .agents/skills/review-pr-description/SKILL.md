---
name: review-pr-description
description: >-
  Review a drafted pull request description in PR_AGENTDESC.md against the branch it would publish, as an independent read-only agent, for unsupported claims, counting, provenance filler, restating the diff, whole-branch fit, and branch coherence. The pr skill's review script runs this as a headless reviewer before every publish, passing the repository root. Use only as a read-only reviewer.
---

# Review a drafted pull request description

Review a drafted pull request description against the branch it would publish, then return a verdict. You're the independent check. You didn't write this description and you didn't watch the work happen, so read what it claims rather than what its author meant to claim. Don't trust a caller's summary of the branch.

## The repository

The caller passes a repository root, and the pr workflow's review script lists it at the end of these instructions.

Treat the first word of the arguments as the repository under review. An empty value means the current working directory.

## Gather the inputs

Read these before judging anything:

- the draft, `PR_AGENTDESC.md`
- the base the draft's `base:` field names, resolved to `origin/<base>` where origin has a copy and to the local branch only where it doesn't, then the merge base of that ref and `HEAD`
- the commit messages that would land, from the merge base to `HEAD`
- the paths and the diff from the merge base to `HEAD`
- `.github/pull_request_template.md` for what each section is asking

Compare against origin's copy of the base rather than a bare branch name. A stale local branch counts commits that already merged as landing here, and you'd flag them as unsupported.

When the pr workflow's review script runs you, all that arrives as files from one snapshot, already resolved against the right base, and the list at the end of these instructions names them: `range.txt` says which base ref and commits the snapshot used. Read the files rather than the live repository.

A missing or empty draft, or a branch with nothing to land, is itself a finding. Report it and stop.

Treat the points under "Settled by the operator" as decided, and don't raise findings against them. Report anything outside the note's scope, or any contradiction between the branch and the note.

## What's already settled

The writer runs a mechanical validator until it reports a clean run before handing you anything, and the caller runs it again afterwards, so leave its rules alone. It has already settled:

- the frontmatter structure, and the title's length and Conventional Commits form
- section presence, section order, and sections left empty
- surviving template comments, unclosed fences, and links pointing nowhere
- whether every path the draft puts in backticks exists

Repeating any of that wastes the round trip.

Read the diff instead. Your job starts where looking at the file stops.

## What to check

Work these four groups. Quote the exact offending text in each finding and give the fix.

### Truthfulness

The description covers this branch and no other.

- Flag any claim the diff doesn't support. A description that mentions a behavior, a flag, or a guarantee absent from the diff is wrong, whatever else it gets right.
- Flag counts of any kind: files, lines, tests, commits, percentages. A count goes stale the moment other work merges, and it pads a description rather than informing it.
- Flag results nobody verified. The Verification section names commands, so check that the commands suit the change. A Go change verified only by a Markdown linter reports nothing about itself.
- Flag a Risk section that says nothing risks anything when the diff touches behavior, and flag invented risk when the diff touches documentation alone.

### Substance

Each section addresses its own question, and the template says which.

- Flag a Summary that restates the diff. `Adds a helper to foo.go and calls it from bar.go` describes a diff the reader can already read.
- Flag a Why that repeats the Summary in longer words, or that doesn't explain anything a reader couldn't recover from the diff.
- Flag provenance filler: requests, review rounds, sessions, prompts, iterations, models, assistants, tools. None of that belongs in a description.
- Flag selling: `robust`, `comprehensive`, `significantly`, `seamlessly`, `powerful`, `elegantly`, and their neighbors.

### Fit

The description covers the whole branch, not the commit its author happened to write last.

- Flag a title describing one commit when the branch has more than one. That title becomes the squash subject on the default branch, so it has to describe what the whole branch does.
- Flag a Summary that covers some commits and drops others.
- Flag labels that misreport the change: a documentation label on a branch touching behavior, or a bug label on a branch fixing nothing.
- Flag `draft: false` on a branch whose own description says the work is unfinished.

### Coherence

Judge the branch, not the description.

- Does the branch carry one change that a reviewer can understand as a whole and that merges or reverts as a unit?
- Flag a branch mixing unrelated work, and list the pull requests it should split into. Reviewers miss things in a branch that changed three unrelated areas.
- A change plus its tests, or a change plus the documentation describing it, counts as one change. Leave that alone.
- Size alone isn't a finding. A wide mechanical rename counts as one change. A small diff touching two unrelated subsystems doesn't.

## Sweep before you write the verdict

A fault you found once is rarely alone. Take each finding you have and read the whole draft again for the same fault. A list that stops short of the diff, a claim scoped wider than the evidence, a count standing where a name belongs, a sentence selling rather than saying. Where the draft holds one, look for the second. Where instances share a fix, report them as one finding that lists each.

Do this even when a finding sends you back into a section you already cleared. The draft in front of you changed since the last pass, and a claim that held then may not hold now.

An instance you leave for the next pass costs a writer round and a reviewer round to say what this verdict could have said. None of that invites padding. Everything in the verdict still has to be something you can point at.

## What to return

Your verdict is what lets the pull request proceed. The pr workflow's review script reads the line below and stamps the draft you cleared. Keep the wording exact, because a verdict that script can't parse leaves the pull request blocked. Don't write to disk, don't run a command that changes anything, and don't edit the draft to make it pass. Reporting the finding is the job.

Return the verdict block and stop there. Skip the preamble, the diff summary, and the praise.

```text
VERDICT: PASS
```

or

```text
VERDICT: CHANGES REQUIRED

1. [truthfulness] <what is wrong>
   text: <the exact offending text>
   fix:  <the specific correction>

2. [coherence] <what is wrong>
   text: <the exact offending text>
   fix:  <the specific correction>
```

Tag each finding `truthfulness`, `substance`, `fit`, or `coherence`.

Return `PASS` when the draft holds up. A clean description counts as a real outcome, and inventing a finding to look thorough wastes the round trip. Report only what you can point at in the draft or the diff.

Your findings go back to the drafting session or its optional writer, so write each one so the recipient can act on it without seeing this conversation. Identify the section and quote the offending text, then give the replacement.
