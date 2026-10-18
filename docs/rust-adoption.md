# Adopt the Rust gates

Include `.devtools/tasks/rust.toml` in `task_config.includes` and set `DEVTOOLS_DIR` through the shared environment drop-in. Rust tools stay in the consumer's `mise.toml`: putting them in shared conf.d would install them in every consumer. Copy this block, choose the Rust version to match your oldest supported Rust version, and run `mise install`:

```toml
[tools]
rust = { version = "1.97.1", profile = "minimal", components = "clippy,rustfmt,llvm-tools" }
"github:EmbarkStudios/cargo-deny" = "0.20.2"
"aqua:nextest-rs/nextest/cargo-nextest" = "0.9.148"
"github:Boshen/cargo-shear" = "1.14.0"
"github:taiki-e/cargo-llvm-cov" = "0.9.1"
"cargo:cargo-mutants" = "27.1.0"
"cargo:cargo-fuzz" = "0.13.2"

[env]
DEVTOOLS_RUST_COVER_LINES = "90"
DEVTOOLS_RUST_COVER_REGIONS = "85"
DEVTOOLS_RUST_FUZZ_TOOLCHAIN = "nightly-2026-08-01"
```

These versions match the source baseline in avirt at commit `214ff16`. Renovate's built-in Cargo and mise managers track consumer manifests and these tool pins, and the shared presets don't need changes. Commit the consumer's mise lockfile and Cargo.lock.

The tasks resolve executables from PATH and report any missing tool. Rustfmt and clippy come from the selected Rust toolchain. Coverage also needs its llvm-tools component.

Copy the templates from `.devtools/config/rust/` into local policy, removing `.template`. Put `clippy.toml`, `deny.toml`, and `rustfmt.toml` at the repository root and `nextest.toml` under `.config/`. Append `workspace-lints.toml.template` to your workspace Cargo.toml, and add `[lints]` with `workspace = true` to each member manifest. Set clippy's `msrv` to match your Rust pin. Review the strict lint catalog when updating Rust: lint names and applicability change between toolchains.

The starter denies clippy's `allow_attributes` and `allow_attributes_without_reason` lints. Use `#[expect(lint, reason = "...")]` for a suppression, with a reason explaining why the code needs the exception. A bare `#[allow(...)]` fails. An item-level allow still fails when it has a reason. Crate-level allows are exempt from `allow_attributes`, but still need a reason. Prefer expectations so exceptions expire. The starter denies `unfulfilled_lint_expectations`. This makes the gate fail once you remove the triggering code. Remove the expectation when fixing that code.

```rust
#[expect(clippy::missing_const_for_fn, reason = "Keep this API non-const until its implementation settles")]
fn identity(value: u8) -> u8 {
    value
}
```

The deny template checks all unmaintained dependencies and denies duplicate versions. A consumer can choose `unmaintained = "workspace"` to restrict that check to direct workspace dependencies, or `multiple-versions = "warn"` when it accepts duplicates. The starter doesn't include avirt dependency exceptions. Review clippy thresholds and selected restriction lints against your API policy.

The nextest starter has zero retries. It fails flaky results and leaked processes and writes JUnit output for the `ci` profile. Adjust its slow-test timeout to your suite.

| Local task | Shared task |
| --- | --- |
| `build` | `devtools:build-rust` |
| `test-rust` | `devtools:test-rust` |
| `lint-rust-all` | `devtools:lint-rust-all` |
| `lint-rust` | `devtools:lint-rust` (clippy) |
| `lint-rustdoc` | `devtools:lint-rustdoc` |
| `lint-shear` | `devtools:lint-shear` |
| `deny` / `vuln` | `devtools:deny-rust` / `devtools:vuln-rust` |
| `fmt` / `fmt-check` | `devtools:format-rust` / `devtools:lint-rust-fmt` |
| `cover-rust` | `devtools:cover-rust` |
| `coverage-report` | `devtools:cover-rust-report` |
| `mutate-rust` | `devtools:mutate-rust` |
| `install-fuzz-toolchain` | `devtools:install-fuzz-toolchain-rust` |
| `fuzz-rust` / `fuzz-smoke` | `devtools:fuzz-rust` / `devtools:fuzz-rust-smoke` |

Local wrappers can use `extends` with these shared names. Replace avirt's corresponding command bodies with wrappers. Remove its nightly installer body and rename `AVIRT_FUZZ_TOOLCHAIN` to `DEVTOOLS_RUST_FUZZ_TOOLCHAIN`. Keep image builds, workloads, `okf-check.py`, coverage floor values, platform choices, fuzz targets, and project policy local.

Compose bare `lint`, `test`, `cover`, `mutate`, and `fuzz` tasks with ordered `run` arrays, including each language's suffixed task. `devtools:lint-rust-all` runs format checking, clippy, rustdoc, shear, and deny in order. `devtools:test-rust` runs nextest, then doc tests. `NEXTEST_PROFILE=ci` selects the CI profile. Extra test arguments go to nextest only. Other tasks forward arguments to their final command. Coverage always measures the whole workspace before reporting.

Coverage runs nextest with `--no-report`, then reports separately to avoid the combined invocation crash. `devtools:cover-rust` requires both coverage floors. `devtools:cover-rust-report` writes HTML without a floor.

Mutation testing accepts surviving mutants and timeouts only when an outcomes file exists. Tool errors still fail. Archive `mutants.out/` in scheduled jobs.

Fuzzing requires an explicit nightly pin and installs it with rustup without changing the stable toolchain. Run `mise run devtools:fuzz-rust target -- -max_total_time=60` from the repository root. Targets live under `fuzz/`. The smoke task prints the targets and runs each for `DEVTOOLS_RUST_FUZZ_SECONDS` seconds (default 20). Archive `fuzz/artifacts/` on failures.

The reusable [cargo-deny scan](../.github/workflows/scan-cargo-deny.yml) runs `devtools:vuln-rust` with the consumer's Rust and cargo-deny pins. Call it on a schedule or pull request with a full devtools commit SHA, like the workflows in the [consumer workflow guide](consumer-workflows.md). It needs only contents read permission. Keep the OS matrix and its single combined check local for now. Platform choices and coverage baselines differ between consumers.
