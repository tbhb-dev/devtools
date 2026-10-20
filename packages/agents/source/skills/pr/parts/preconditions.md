## Preconditions

This skill assumes the shared tbhb toolchain:

- `gh` installed and authenticated
- a `mise run lint-pr-description` task
- combined lint and test tasks named by `lint_task` and `test_task` in `.agents/agent-skills.conf`, defaulting to `lint` and `test`
- a gitignore entry for `PR_AGENTDESC.md`
- a pull request template at `.github/pull_request_template.md`
- the `review-pr-description` and `fix-prose` skills deployed alongside this one
- the `watch-pr`, `fix-pr`, and `merge-pr` skills deployed for step 8

Preflight checks each. Where one is missing, tell the operator rather than improvising a substitute.
