# Release version literals

Run `mise run check-versions` to check the eleven published release literals, including the Python package's entry in `uv.lock`. The checker also matches each of the four local APM lock entries to its package manifest by local path. It uses `git describe --tags --abbrev=0` to select the nearest reachable tag. Without a reachable tag, it fails with guidance to fetch tags and the complete history.

An explicit version overrides tag discovery. These commands check the same version, even without tags:

```bash
bash tools/check-versions.sh 1.2.3
bash tools/check-versions.sh v1.2.3
```

The compatibility script forwards to `mise run run agentrepo check-versions`, which builds from this repository's working tree. Release readiness, release verification, and lint keep calling that script. Success is quiet. Stale sites print their file, line, expected text, and found text, followed by `TOTAL: <n> finding(s) against v<version>` and exit status 1. Missing sites and read failures also fail the check.

No consumer impact: the script and build task stay outside the vendored and APM payloads.
