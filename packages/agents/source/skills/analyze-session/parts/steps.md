## Step 1: read the lines the preflight listed

The preflight listed the failing lines and the line ranges of each repeated sequence. Open only those:

```text
scripts/session.py grep <transcript> 'pattern' --context 300
scripts/session.py show <transcript> 361,408,1938
```

`grep` searches the decoded content blocks, so what prints matches what the model saw. Add `--kind THINK` to read what the agent believed at the time, which is often where an error begins. `show` prints whole records, once a report has given you a specific line number.

Either harness can have written the transcript. Whichever harness runs this skill, the tool detects the format from the file, and the `index` report in the preflight output names it. When detection fails, pass `--format claude` or `--format codex`.

## Step 2: test the session's own conclusions

A transcript records what an agent believed, which isn't always what was true. Treat every claim in it as a hypothesis with a line number attached, including the agent's own corrections.

Re-run the decisive command wherever that environment still exists.

**Check that a test can fail before trusting a result of zero.** A test returning nothing may mean the thing is absent, or may mean the test stopped examining anything. Give it a case that must produce output, and treat an empty result as evidence only after that check passes. A test that has stopped examining anything doesn't report anything at all, where a working one still returns unrelated findings alongside the empty answer.

## Step 3: verify every figure with three agents

Readers trust a number more than a description, so a wrong count does more damage than a wrong impression. Report a figure to the operator only after independent replication.

Follow `references/VERIFY_FIGURES.md`. It gives the claims table format, the prompt each verifier receives, the isolation rule for verifiers that write files, and how to settle a figure they disagree on. `references/DISPATCH.md` says how to launch the agents.

Don't report a figure until all three report. A verifier that hasn't answered yet isn't an abstention, and counting it as one discards the report most likely to disagree.

## Step 4: recommend fixes across the five areas

Step 3 runs identical prompts, because replication gains nothing from a division of labor. This step does the opposite. It gives each agent one area to keep the recommendations distinct.

Follow `references/FIX_SURFACES.md`. It lists the five areas, the shared prompt, and the block each agent receives.

Synthesize the results yourself. Each agent sees one area, while this session has the verified figures and the full set of findings that no agent had alone.

## Step 5: write the retrospective

Fill in the `RETRO.md` the preflight created, following `references/WRITING_THE_RETRO.md`.

Cite a line number for every claim. Rank findings by round trips wasted rather than by how irritating they were. State what each fix costs as well as what it improves, and give the operator the retro path when you report back.

## Preconditions

This skill assumes the shared tbhb toolchain:

- `uv` on PATH, which the `session.py` shebang uses to select an interpreter
- a git repository, because the retro directory resolves from the main checkout
- a transcript path, or a session ID matching exactly one file

Preflight checks each. Where one is missing, tell the operator rather than improvising a substitute.
