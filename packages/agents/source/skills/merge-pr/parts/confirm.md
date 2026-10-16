The merge script settles who confirmed the merge, in this order, and prints which one it took:

1. `--confirmed`, which you pass only after the operator answered yes to this message in this session
2. a merge pre-approval for this session, which the operator granted through `mise run preapprove merge` and preflight reported as `merge: GRANTED`
3. the machine's delegation check, when preflight reported one configured: it answers for the repository and the action `merge`, and a refusal stops the merge

A bare `mise run preapprove` grants commit and pr and never merge, because a merge is the one action here no later step can undo. With none of them the script exits 75 and merges nothing. Ask the operator, then run it again with `--confirmed`.

A grant or a delegation answers one question, merge under this message. Ask the operator anyway where any of these holds:

- the review returned a finding nobody acted on
- the checks aren't green
- this pull request isn't the one the session set out to merge, which covers a dependency bump nobody here authored unless the operator named it
