## Preconditions

This skill assumes the shared tbhb toolchain:

- `gh` installed and authenticated
- a `mise run lint-pr-description` task
- a gitignore entry for `PR_AGENTDESC.md`
- a pull request template at `.github/pull_request_template.md`
- the `review-pr-description` and `fix-prose` skills deployed alongside this one
- the `watch-pr`, `fix-pr`, and `merge-pr` skills deployed for step 8

Preflight checks each. Where one is missing, tell the operator rather than improvising a substitute.

Preflight and publication refuse more than 800 source lines of code or 2,000 prose lines against the draft's publish base. Split by concern, along commit boundaries, or as a stacked series. Only Renovate bot authors (`app/renovate` or `renovate[bot]`, with `is_bot`) are exempt. The checks measure `chore` titles and other bots. Install `agentrepo` v0.23.0 with `size check` and the pinned `scc` before adopting these skills. Missing size capability stops publication. The size limit has no override.
