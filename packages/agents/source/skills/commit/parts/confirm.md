The commit script settles who confirmed the commit, in this order, and prints which one it took:

1. `--confirmed`, which you pass only after the operator answered yes to this message in this session
2. a pre-approval for this session, which the operator granted through `mise run preapprove` and preflight reported as `commit: GRANTED`
3. the machine's delegation check, when preflight reported one configured: it answers for the repository and the action, and a refusal stops the commit

With none of them the script exits 75 and commits nothing. Ask the operator, then run it again with `--confirmed`.

A grant or a delegation answers one question, commit this message, and says nothing about whether this is the right commit. Ask the operator anyway where any of these holds:

- the review returned a finding nobody acted on
- a gate needed more than a mechanical fix
- the grouping changed after step 2
- the commit goes beyond what the session set out to do

The operator withdraws a grant with `mise run revoke-preapproval`. Say so where a run keeps arriving at one of those exceptions.
