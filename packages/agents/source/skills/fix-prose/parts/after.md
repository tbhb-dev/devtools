The review exists because a clean lint run is a weak result on its own, and no gate scores a claim that moved, a name traded for a vaguer phrase, or a metaphor that arrived while somebody was clearing a rule. A document can pass every checker and come back saying something its author never wrote.

Act on its verdict:

- `VERDICT: PASS` ends the loop. Go on to the final lint.
- `VERDICT: CHANGES REQUIRED` returns to step 2 with the findings verbatim in the notes file.

Bound it at three review rounds. Stop after that and report which findings keep returning, because a fourth pass at the same objection rarely helps. Run the verification in step 3 again after every writer round.

## Sanctioning a dictionary word

The writer never edits a dictionary. A token it reports as unresolved waits for a decision. When the operator decides the word belongs, add the line to `.cspell-words.txt` or to the vale vocabulary's `accept.txt`, then record exactly that line against the run:

```text
check-suppressions.sh --allow <state> .cspell-words.txt <word>
```

The verification then passes on those added lines and still fails on anything else, including a removed word, an extra word, or an edit to a rule file. The operator can sanction lines in only those two dictionary files. Do it only on the operator's word, never on the writer's suggestion.

## Final lint and cleanup

Run the complete lint command once more, with its output discarded, and read only its exit status. Whether the gates pass is the question here, and a finding printed in this context defeats the point of the skill.

Always run `check-suppressions.sh --cleanup <state>` before returning, once no agent needs the state, even when the outcome is unresolved or blocked.

## What to report

Report `PROSE: CLEAN` only when the writer reported clean, the suppression check is silent, the reviewer returned `VERDICT: PASS`, and the final lint command passes. Otherwise return the unresolved or blocked report and nothing else.

Each unresolved entry needs a decision that changes what the document claims, which is yours or the operator's rather than another lint round. A rule matching correct text arrives on that list by design. The writer never turns a rule off or adds an exception, and never rewrites a sentence into nonsense to clear one.

## Safety rules

- Only the writer edits the target, and the target is the only file it may edit.
- Never change lint configuration, dictionaries, hooks, or suppression comments on the writer's behalf.
- Don't infer clean from silent output unless the preflight proved the target has prose rules.
- Don't print full lint findings in this session unless the workflow blocks and the operator needs a decision.
