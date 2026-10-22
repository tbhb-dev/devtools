# Consumer CI and security workflows

Call the reusable workflows from jobs in your repository. Pin each external `uses` reference to the full devtools release commit SHA. The workflows run on `ubuntu-24.04`, check out the caller, turn off persisted checkout credentials, harden the runner and enforce `MISE_LOCKED=1`. They use the caller's committed mise configuration and lockfiles.

| Workflow | Caller job ID | Required token permissions |
| --- | --- | --- |
| `consumer-lint.yml` | `lint` | `contents: read` |
| `scan-cargo-deny.yml` | `cargo-deny` | `contents: read` |
| `scan-gitleaks.yml` | `gitleaks` | `contents: read` |
| `scan-govulncheck.yml` | `govulncheck` | `contents: read`, `security-events: write` |
| `scan-gomodscan.yml` | `gomodscan` | `contents: read`, `security-events: write` |

Replace `DEVTOOLS_RELEASE_COMMIT_SHA` below with the release's full commit SHA before running actionlint:

```yaml
permissions:
  contents: read

jobs:
  lint:
    uses: tbhb-dev/devtools/.github/workflows/consumer-lint.yml@DEVTOOLS_RELEASE_COMMIT_SHA
  cargo-deny:
    uses: tbhb-dev/devtools/.github/workflows/scan-cargo-deny.yml@DEVTOOLS_RELEASE_COMMIT_SHA
  gitleaks:
    uses: tbhb-dev/devtools/.github/workflows/scan-gitleaks.yml@DEVTOOLS_RELEASE_COMMIT_SHA
  govulncheck:
    uses: tbhb-dev/devtools/.github/workflows/scan-govulncheck.yml@DEVTOOLS_RELEASE_COMMIT_SHA
    permissions:
      contents: read
      security-events: write
  gomodscan:
    uses: tbhb-dev/devtools/.github/workflows/scan-gomodscan.yml@DEVTOOLS_RELEASE_COMMIT_SHA
    permissions:
      contents: read
      security-events: write
```

Keep caller job IDs `lint` and `test` stable for the fix-pr routing contract. Keep repository-specific test jobs in the caller. Reusable jobs add a called-job component to GitHub's displayed check names. Check required-status rules when adopting them. devtools retains its own lint setup because its release and private-identifier gates differ from consumer setup, and calls the reusable security jobs locally.

The lint workflow requires `devtools:check-vendored`, `install-tools`, `install-guard-markdown` and `lint`. It checks vendored payload first, installs project dependencies and the Markdown guard, then adds the Go binary directory to PATH before linting. Checkout includes full history.

With full history checked, the gitleaks workflow invokes `mise run devtools:gitleaks -- --mode ci`. It scans all fetched refs, including unrelated pushed branches and tags, regardless of the checked-out ref. Scanner failures fail the job. Your root gitleaks configuration still applies. See [the fixture policy](gitleaks.md) for the default policy and narrow fixture exceptions.

Go scans require the shared Go tasks and scanner tools declared in the caller's module. SARIF tasks report findings without converting them into scanner execution failures. Task errors still fail the job. Keep the separate reachable-vulnerability gate (`mise run devtools:vuln`) in CI when findings must fail checks. Run security workflows on pull requests, pushes to the default branch and a daily schedule to seed code scanning and detect new advisories.

Update the vendored `.devtools/scripts/filter-govulncheck-sarif.sh` with the workflow release. Running the govulncheck workflow requires this helper and `jq`. The supported runner provides `jq`. Preserving SARIF metadata, the helper retains only results with nonempty `codeFlows` for code scanning, including an empty result list to clear previous alerts. The untouched raw report remains in the `govulncheck-raw-sarif` artifact for 14 days, including after a scanner failure if a report exists. Filtering, artifact, and upload failures fail the job.

Under its own category, the gomodscan report remains unfiltered.

Run `mise run devtools:lint-workflows` and `mise exec -- bats tests/workflows/security.bats tests/workflows/sarif.bats` in devtools to verify the workflow contracts and SARIF fixtures.

The Rust advisory workflow runs `devtools:vuln-rust` and fails on findings or scanner errors. The caller includes the shared Rust tasks and pins Rust and `cargo-deny` in its own mise configuration. See [Rust adoption](rust-adoption.md) for the tool block and starter policy.

The text `devtools:gomodscan` task and `devtools:vendor-check` intentionally skip modules without a tracked `vendor/modules.txt`, with a skip diagnostic. They don't create a vendor directory for these modules. Vendored modules keep the scan and vendor drift checks. The SARIF task requires a tracked vendor manifest. Non-vendoring modules must omit the gomodscan upload workflow.

The wrappers support the pinned gomodscan v0.1.0 completion diagnostics. Any extra stderr output, including a module lookup failure, or a missing completion summary is a tool failure (status 2). Diagnostics keep the module and service error. Complete text scans return 0 when clean and 1 for findings. Complete SARIF scans return 0 for either outcome, after validating the report's JSON, SARIF version, runs, tool names, and result messages. SARIF validation requires `jq`. The output parent directory must already exist. An output open failure or invalid report fails the task with the requested path in the error.

Agent content publication also requires the `gh-post` tool pin and machine configuration. See [the GitHub posting guide](gh-post.md) for installation, allowed identities, private-string loading, and exit codes. Updated skills stop when the command is missing.

When adopting the scan modes, sync `.devtools/scripts/` with `.devtools/tasks/` and update the skill package together. Fetch complete origin history and set `refs/remotes/origin/HEAD` before local publication. PR publication also needs the declared origin base. Keep security aggregates and CI on explicit `--mode ci`. See [scan boundaries](gitleaks.md#scan-boundaries) for local coverage and index semantics.
