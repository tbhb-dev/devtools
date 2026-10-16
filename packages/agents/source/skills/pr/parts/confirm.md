`create-pr.sh` settles who confirmed the publish, in this order, and prints which one it took:

1. `--confirmed`, which you pass only after the operator answered this step's question in this session
2. a pre-approval for this session, which the operator granted through `mise run preapprove` and preflight reported as `pr: GRANTED`
3. the machine's delegation check, when preflight reported one configured: it answers for the repository and the action `pr`, and a refusal stops the publish

With none of them the script exits 75 and publishes nothing. Ask the operator, then run it again with `--confirmed`.

Where preflight reported `pr: GRANTED`, take the answers from the grant rather than from a question:

| Preflight line | How far to take it | How fixes land |
| --- | --- | --- |
| `pr: GRANTED`, `merge: GRANTED` | `Open, fix, and merge` | Separate commits |
| `pr: GRANTED`, `merge: not granted` | `Open, watch, and fix` | Separate commits |

Separate commits are the pre-approved choice because amending rewrites a commit the operator already cleared, and a grant covering that would go beyond the question it answers. Amend only where the operator asks for it in the session.

A grant or a delegation answers one question, publish this description, and says nothing about whether the branch is right. Ask the operator anyway where any of these holds:

- the review returned a finding nobody acted on
- a gate needed more than a mechanical fix
- the branch goes beyond what the session set out to do
- the push would rewrite the branch on origin
