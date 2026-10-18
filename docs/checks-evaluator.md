# Saved check evaluation

`agentrepo checks evaluate` reads a saved `gh pr view --json statusCheckRollup` object from stdin, or from `--file PATH`, and writes a JSON result. It doesn't make network requests.

To verify a head, save `gh pr view --json headRefOid,statusCheckRollup` and pass `--expected-head SHA`. A missing head field is unreadable input when the caller expects a head.

Repeat `--required NAME` for each required check. Names match exactly. Punctuation and spaces count toward the match. Every selected workflow with that name must report `SUCCESS`. Rule discovery and ruleset include/exclude patterns belong to callers. This command accepts the resulting names.

The command groups check runs by workflow and check name. Status contexts form separate groups by context name and use `createdAt` for ordering. The command selects the newest entry. An absent or zero start time on a check run represents a rerun that hasn't started and sorts after started runs. Equal timestamps choose failure over pending over success, regardless of input order. Missing status timestamps sort after known timestamps. Entries with missing timestamps tie and use the same conservative verdict rule.

General checks accept `SUCCESS`, `NEUTRAL`, and `SKIPPED`. Required checks accept only `SUCCESS`. Known unfinished check states are pending. Unknown states or conclusions fail. Malformed JSON, absent or null rollup values, unknown entry types, missing entry names, and invalid timestamps are unreadable input.

| Exit status | Outcome |
| --- | --- |
| 0 | Success |
| 1 | Failed check |
| 2 | Pending check |
| 3 | Missing required check |
| 4 | Empty rollup |
| 5 | Unreadable input or invalid command arguments |
| 6 | Head moved |

Results contain the numeric `outcome`, sorted unique `names` when checks contribute to the outcome, and an `error` for unreadable input. When more than one condition applies, precedence is unreadable input, head moved, empty rollup, missing required checks, failed checks, pending checks, then success. File and stdin read errors also return status 5. JSON output errors return status 5 through the command error path.

This is a new interface. Existing merge and watch scripts behave as before until they adopt explicit compatibility adapters.
