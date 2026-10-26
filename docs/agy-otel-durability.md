# Spool durability finding

This finding answers [#463](https://github.com/tbhb-dev/devtools/issues/463). It analyzes the hook in [#442](https://github.com/tbhb-dev/devtools/issues/442), the flush protocol in [#443](https://github.com/tbhb-dev/devtools/issues/443), and metrics in [#456](https://github.com/tbhb-dev/devtools/issues/456) through a benchmark that doesn't change production behavior.

Decision: accept process-crash guarantees only (option a).

Lost or duplicated telemetry after a machine crash is acceptable here. The spool stores only telemetry. Directory barriers add synchronous storage work to a hook that blocks the agent, while the proposed protocol still deliberately loses counts between intent markers and state writes. File and directory syncs can't make a collector acknowledgement atomic with a local marker either. Stronger storage ordering would reduce machine-crash windows without delivering exactly once telemetry. Keep the existing process-crash contract and revisit this choice if telemetry becomes an audit log.

## Ordering analysis

The table analyzes the filesystem protocol. "Yes" means a candidate OS-crash protocol needs an explicit barrier at that step. The recommendation preserves the process-crash contract. "Already" identifies a barrier the issue contract already specifies. Directory sync means opening that directory and calling `fsync` on its descriptor, checking the result before the next dependent mutation. A failed barrier must leave recovery evidence intact.

Linux's [sync manual](https://www.man7.org/linux/man-pages/man2/fsync.2.html) requires a separate directory sync for entry persistence. The [ext4 documentation](https://cdn.kernel.org/doc/html/latest/admin-guide/ext4.html) describes delayed allocation and journal ordering, whose defaults still require apps to use barriers across separate operations. Apple's [storage guidance](https://developer.apple.com/documentation/xcode/reducing-disk-writes) distinguishes ordinary sync from a stronger drive-cache flush. On macOS, plain `fsync` doesn't establish a universal power-loss guarantee. Future durability code must also assess `F_FULLFSYNC` and the storage stack. These measurements use ordinary directory `fsync`, not that stronger operation. A Linux VM also depends on its host honoring flush requests. Neither host was power-cut.

| Step | Barrier needed? | Ordering and prevented failure |
| --- | --- | --- |
| Create spool root and subdirectories | Yes | Sync each new directory and its parent, working outward through any newly created ancestors, before accepting durable records. Otherwise a surviving file can lose its reachable directory tree. |
| Create and write hook temporary record | Already | Sync the file before publication to prevent an entry pointing at incomplete contents. Skip temporary-directory sync before publication because the hook may drop an unpublished event. |
| Rename hook record from `tmp/` to `new/` | Yes | Sync `new/`, then `tmp/`, before reporting durable publication. This persists the destination and source removal before a later consumer moves or removes the record. Otherwise a published event can vanish or return in the source directory. |
| Remove failed or stale temporary record | Yes | Sync `tmp/` after unlink if deletion must survive reboot. Otherwise cleanup can drop and count a resurrected temporary record again. Deletion needs only the directory barrier. |
| Take flush lock | No | Because the kernel releases locks on reboot, syncing lock contents doesn't help exclusion. The first row covers directory creation. |
| Read, group, pair, and build records | No | These steps only read persistent state, so they don't need barriers for missing partners or stable IDs. |
| Create unsealed claim directory | Yes | Sync `cur/` before moving records into the claim so the recovery directory can't disappear while source removals survive. |
| Move records from `new/` into claim | Yes | After the moves, sync the claim directory, then `new/`, before sealing. Skip another file-content sync because the hook already synced each immutable record. This prevents a durable seal on an incomplete claim or source resurrection after disposal. |
| Seal claim by renaming within `cur/` | Yes | Sync `cur/` before build, counting, or export. Otherwise sealing can roll back after downstream actions survive. |
| Recover unsealed claims | Yes | Move records back, sync `new/`, then the claim directory before removing it. Sync `cur/` after removal. Otherwise recovery can lose a moved record or resurrect an already recovered claim. |
| Record resolution or signal shutdown | Already | Create the `.done` file with `O_EXCL`, then sync it and the batch directory before dependent deletion. Otherwise an accepted or turned-off signal can cause another export. A response before its marker remains an allowed resend window. |
| Record expiry | Already | Sync `expired` and its batch directory before disposal. Otherwise reboot can forget expiry and resume export. |
| Begin disposal | Already | Sync `dispose` and the batch directory before deleting records. Otherwise recovery can rebuild a partly deleted batch without its disposal intent. |
| Delete batch records | Yes | Sync the batch directory after removing all records, before deleting any guard markers. Otherwise a guard can disappear while recovery rebuilds or counts a remaining record again. |
| Delete other markers | Yes | Sync the batch directory before deleting `dispose`. Otherwise resolution or counting markers can reappear without their disposal intent. |
| Delete `dispose` | Yes | Sync the now-empty batch directory before removing it. Otherwise directory removal can roll back to an apparently unfinished batch. The documented empty-directory recovery still applies. |
| Remove batch directory | Yes | Sync `cur/` after removal. Otherwise disposed directories can reappear. Recovery of empty or dispose-only directories uses the same ordered deletion. |
| Create count guard | Already | Sync each exclusive count marker and its parent before updating totals. Otherwise recovery can apply totals again without their guard. |
| Create metrics guard | Already | Sync `metrics.counted` and its batch directory before totals. The intentional marker-before-state loss window remains, even with all barriers. |
| Replace counts or retry state | Yes | Sync the temporary file, rename over the target, then sync `state/` before relying on the replacement. The issues already require file sync. The directory barrier prevents old counts or retry deadlines returning after reboot without a separate temporary entry sync. |
| Replace metrics state, including permanent rejection | Already | Sync the temporary file, rename, sync `state/`. This keeps the version, totals, start time and rejected version in one durable replacement. It can't couple them atomically to a remote response. |
| Trim retained records | Yes | Persist the count guard before the state update, and sync `new/` after unlink, before removing any associated guard. Otherwise a deleted record can return and trigger counting or export again. Guard location and lifetime need definition in a future durability design. |
| Reset state after external loss | No | No sync can recover deliberately removed or lost state. A new metrics start time denotes the reset. |
| Network export, deadline, and retry calculation | No | Sync the persistent markers and state replacements described in this table, since pure computation and network operations don't need a filesystem barrier. |

Destination-first ordering is conservative across directory moves. This doesn't claim that partial rename persistence occurs on these mounts. Future code must check filesystem-specific recovery behavior and handle duplicate names during interrupted claim recovery, since directory barriers alone don't supply that policy.

## Measurement method

[The benchmark](../tools/durability/spool_test.go) uses Go's standard library. Run it on the filesystem used for the spool, setting `TMPDIR` if necessary:

```sh
go test ./tools/durability -run '^$' -bench BenchmarkHook -benchtime=1000x -count=3
go test ./tools/durability -run '^$' -bench BenchmarkFlush -benchtime=30x -count=3
```

The hook model writes a small sanitized JSON record with exclusive creation, file sync, close, and rename. Its candidate adds destination and source directory syncs. It measures filesystem work in process, excluding binary startup, payload decoding, and hashing. The hook's 20 ms whole-process budget provides context for the added p99, though these numbers don't certify that budget.

The flush model claims and seals a 100-record batch. It writes counting markers and state replacements before resolving traces and logs, then disposes the batch. Existing marker directory syncs and the metrics-state directory sync remain in the baseline. The candidate adds claim, seal, counts/backoff replacement and disposal barriers from the table. It batches record-move and unlink barriers at phase boundaries. Both modes start with durably seeded records. Timing excludes seeding and temporary root cleanup. State replacements persist between passes. The benchmark excludes the network and payload building as well as retention, expiry, and crash recovery. The difference estimates added filesystem time for this flush path. It can't predict the runtime of the future exporter.

Runs repeat on live agent hosts without dropping caches or changing mount settings. They report the spread rather than treating a busy host as a controlled storage laboratory. `ns/op` is mean per operation, and `p99-ns` is the nearest-rank hook p99 within that run. Raw results follow.

## macOS measurements

- Date: 2026-10-26
- System: macOS 27.0.1 on Apple M3 Max
- Compiler version: `go1.27.3`
- Storage uses Apple File System, confirmed locally by GNU `stat`

Runs were serial. An exploratory overlapping run prompted this repeat, which measures hook and flush operations separately without that competing workload.

```text
BenchmarkHook/directories=false-16         	    1000	   6532450 ns/op	   9135708 p99-ns
BenchmarkHook/directories=false-16         	    1000	   5544974 ns/op	   9948292 p99-ns
BenchmarkHook/directories=false-16         	    1000	   5465511 ns/op	   8989750 p99-ns
BenchmarkHook/directories=true-16          	    1000	  16948932 ns/op	  22616083 p99-ns
BenchmarkHook/directories=true-16          	    1000	  17504188 ns/op	  22859916 p99-ns
BenchmarkHook/directories=true-16          	    1000	  17031664 ns/op	  21990375 p99-ns
```

Hook p99 spans 8.990 to 9.948 ms before adding directory barriers and 21.990 to 22.860 ms afterward. Corresponding repetitions add 12.912 to 13.480 ms, with a difference of medians of 13.480 ms. Filesystem work alone exceeds the whole hook's 20 ms p99 target in the candidate runs.

The macOS flush repeat used `-benchtime=5x`, while Linux used the command shown earlier:

```text
BenchmarkFlush/directories=false-16         	       5	 292428433 ns/op
BenchmarkFlush/directories=false-16         	       5	  93958491 ns/op
BenchmarkFlush/directories=false-16         	       5	  92301158 ns/op
BenchmarkFlush/directories=true-16          	       5	 145505709 ns/op
BenchmarkFlush/directories=true-16          	       5	 146186983 ns/op
BenchmarkFlush/directories=true-16          	       5	 152665308 ns/op
```

Flush means span 92.301 to 292.428 ms in the baseline and 145.506 to 152.665 ms with the candidate barriers. Subtracting medians gives 52.228 ms added per pass. Corresponding repetitions range from -146.923 to +60.364 ms, including the slow baseline outlier. This spread limits precision. The hook result supports the decision even without treating this flush estimate as a reliable bound.

## Linux measurements

Measurement ran on 2026-10-26. This host used Linux 6.6.87.2 on WSL2 with an Intel i9-13900KF and Go 1.27.3. Storage matters here. `findmnt` confirmed ext4 with `data=ordered` for the benchmark temporary directory, backed by the VM host disk.

```text
BenchmarkHook/directories=false-32             1000   3372324 ns/op   5064538 p99-ns
BenchmarkHook/directories=false-32             1000   3352676 ns/op   5083233 p99-ns
BenchmarkHook/directories=false-32             1000   3343574 ns/op   5306674 p99-ns
BenchmarkHook/directories=true-32              1000   6671431 ns/op  12562195 p99-ns
BenchmarkHook/directories=true-32              1000   6276752 ns/op   7928235 p99-ns
BenchmarkHook/directories=true-32              1000   6370199 ns/op   8860864 p99-ns
BenchmarkFlush/directories=false-32              30  32514767 ns/op
BenchmarkFlush/directories=false-32              30  33492231 ns/op
BenchmarkFlush/directories=false-32              30  32976207 ns/op
BenchmarkFlush/directories=true-32               30  62858809 ns/op
BenchmarkFlush/directories=true-32               30  60426708 ns/op
BenchmarkFlush/directories=true-32               30  59840931 ns/op
```

Hook p99 ranges from 5.065 to 5.307 ms in the baseline and 7.928 to 12.562 ms with directory barriers. Subtracting corresponding repetitions gives an added p99 of 2.845 to 7.498 ms. Median p99 difference is 3.778 ms. Flush means span 32.515 to 33.492 ms before adding barriers and 59.841 to 62.859 ms afterward, adding 26.865 to 30.344 ms per pass across corresponding repetitions.

## Decision

Decision: accept process-crash guarantees only (option a).

Keep file and marker syncs already specified by the work issues. Don't add the candidate barriers to the synchronous hook or claim OS-crash durability. After a machine crash, telemetry can lose events or duplicate exports. Counts may disappear, contributions may repeat, and retry state may roll back. They never lose user data. Reconsidering this decision requires measurements of stronger macOS flush semantics and recovery design as well as the extra directory syncs.

## Follow-through

The non-goals in #443 and #456 link to #463. Their links should point to the findings comment once it exists. Option a calls for those references rather than a durability work issue.
