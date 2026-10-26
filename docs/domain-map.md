# Repository domain map

Snapshot: 2026-10-26 at 20:19 UTC, base `d555364c`. Window: 2026-07-28 at 20:19 UTC through 2026-10-26 at 20:19 UTC. This report covers 333 open issues, 246 merged pull requests, and 39 open pull requests. The [method and checks](domain-map/method.md) explain collection, classification, and offline reproduction. This is a decision input for [#553](https://github.com/tbhb-dev/devtools/issues/553).

## Size and open work

| Domain | Tracked paths | Primary open issues | Merged pull requests crossing its boundary |
| --- | ---: | ---: | ---: |
| Skills | 702 | 173 | 79 |
| Go | 2,147 | 46 | 36 |
| Consumer build tooling | 64 | 32 | 72 |
| Workflows | 23 | 8 | 29 |
| Release | 10 | 29 | 20 |
| Shared | 154 | 45 | 151 |

Go includes 2,028 vendor paths. Its own code and configuration occupy 119. Skills holds most primary open work. The [issue mapping](domain-map/issues.csv.gz) records primary and secondary domains with their evidence. Keyword rules can misclassify broad titles, while body references to root configuration or docs can make shared primary. The mapping records labels without changing GitHub.

## Crossing and conflict

The merged pull requests split into 84 within one domain and 162 across domains. Shared participates in 151 of those crossings. Pair totals peak at shared / skills (78), consumer / shared (68), and Go / shared (34). Among pairs without shared, consumer / skills leads (24), followed by consumer / Go and Go / skills (16 each). [Pair totals](domain-map/pairs.csv) count each pair once per pull request. Its [path table](domain-map/merged-paths.csv.gz) supplies the underlying changes.

The [open conflict view](domain-map/open-conflicts.csv.gz) lists 140 paths that open pull requests share, their domains, and the affected pull requests. It measures overlapping filenames. It doesn't detect conflicting hunks or merge failures.

## Candidate boundaries and recommendation

Each candidate extraction cuts across the merged pull requests in the last column of the preceding table, including shared paths. Those figures overlap. A split would need a decision about shared gates, docs, and root configuration.

**Recommendation: keep the repository together for now.** Most merged work crosses a domain boundary, and shared changes drive that result. Direct crossings between skills and consumer tooling add coordination work that the snapshot doesn't resolve into a clear split boundary. The overlap view can't show whether a split would reduce actual merge conflicts. This report changes documentation alone.
