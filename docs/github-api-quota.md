# GitHub API quota in pull request skills

PR publication, merge, and watch scripts read the active account through GraphQL `viewer.login`. An exhausted REST core quota doesn't prevent that check. Before a push, publication, or merge, the configured account must still match. A failed request prints GitHub's error. Account mismatches report the actual and configured logins.

For a rate-limit error, the shared reader returns status 76 and prints the affected quota and its reset time. It stops without retrying the request. A reset header supplies Unix seconds. Otherwise one REST `GET /rate_limit` reads the relevant resource's reset in Unix seconds without consuming core quota. If that lookup also fails, the message says the reset is unavailable and keeps the original error. Authentication and permission failures keep the original command status rather than becoming an identity mismatch. For ordinary check-read errors, the watcher retains status 2.

## Calls per invocation

These counts describe the explicit readers in the published scripts. GitHub CLI may make extra repository discovery requests or paginate a connection. No reader caches identity across invocations.

| Operation | REST core reads | GraphQL reads |
| --- | --- | --- |
| PR preflight | 0 | Viewer, PR lists, repository, labels, milestones, issues, and published body when comparing a draft |
| PR publication | 0 | Viewer, labels, optional milestones, each distinct referenced issue, final identity check when configured, PR lookup, and current properties for updates |
| Merge preflight | 0 | Viewer reused for the identity report, PR snapshot. PR lookup when the caller supplies no number |
| Squash briefing | 0 | Viewer, one PR snapshot including commits and files, each Related reference. PR lookup when the caller supplies no number |
| Merge review | 0 | Viewer, PR snapshots, base refs, repository URL for fetching pinned objects |
| Merge execution | 0 | Viewer, identity when configured, PR snapshots and base refs for the live gates, final PR URL |
| Watch | 0 | Viewer, optional PR lookup, one checks request per snapshot |

PR writes still use `gh pr create`, `gh pr edit`, and `gh pr ready`. Merge uses `gh pr merge`. GitHub CLI can spend REST quota while resolving reviewers, assignees, labels, and milestones or applying those properties. The scripts don't retry these writes automatically. Git push and fetch use Git transport, outside API quotas. Merge diff review uses pinned local Git objects rather than a REST diff download. The separate fix-pr workflow's Actions run and log readers still use REST through `gh run`. They're outside these read counts.

The rate-limit classifier recognizes GitHub's rate-limit message, including a secondary rate limit. A secondary limit need not end at the primary quota reset: follow any retry guidance in the original error. A bare HTTP 403 without a rate-limit message remains a permission failure.
