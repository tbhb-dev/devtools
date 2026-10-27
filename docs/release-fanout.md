# Release fan-out foundation

Use `internal/fanout` for ledger validation, release verification, deadline decisions, and result summaries under #766. With no consumer writes, timers, background work, or polling, this foundation supplies the inputs for preparation (#767) and publication (#768).

## Ledger version 1

Store the authoritative snapshot at `release/consumers.json` in the release commit. `Load` checks the tag through the existing release verifier before reading that commit's ledger with `git show`. It pins the tag object across verification and reads the annotated tagger timestamp. A changed tag or failed verification returns an empty release. Missing or invalid ledgers also fail. `Load` ignores the checkout's ledger. Membership governance and the initial production ledger belong outside this foundation.

```json
{
  "version": 1,
  "entries": [
    {"repository": "example/consumer", "disposition": "active"},
    {"repository": "example/retired", "disposition": "inactive"},
    {"repository": "example/manual", "disposition": "exempt"}
  ]
}
```

Provide an entries array with at most 100 entries. Each repository name uses `owner/repository` syntax without credentials. Names must be unique without regard to case. Choose `active`, `inactive`, or `exempt` for each disposition. Repeated JSON members and null values cause refusal. Unknown versions and fields cause refusal, as do malformed snapshots and invalid or duplicate entries. Refusals return a fixed error without credentials and with no entries. An empty array passes validation but fails adoption.

## Deadlines and results

`Decide` takes the tagger timestamp T and delivery time. The caller records results within the computed reporting deadline.

| Delivery | Writes allowed | Result | Reporting deadline | Missed adoption target |
| --- | --- | --- | --- | --- |
| Before T + 55 minutes | Yes | Caller supplies result | T + 1 hour | No |
| From T + 55 minutes until T + 1 hour | No | `late-event` | T + 1 hour | No |
| At or after T + 1 hour | No | `late-event` | Receipt + 5 minutes | Yes |

`Summarize` requires exactly one result per ledger entry. It refuses unknown repositories and values, along with duplicate or missing results, and disposition mismatches. Inactive and exempt entries must receive matching results. They don't enter the active denominator.

| Result class | Closed vocabulary |
| --- | --- |
| Adopted | `current`, `existing`, `opened`, `newer` |
| Excluded | `inactive`, `exempt` |
| Failed | `late-event`, `deadline`, `prepare-failed`, `publish-failed`, `access-refused`, `scan-refused`, `conflict`, `uncertain` |

Every active entry must have an adoption result for `Summarize` to return `passed`. Any active failure produces `failed`. A zero active denominator produces `no-active-entries`. Stage details don't add results.
