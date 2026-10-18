#!/usr/bin/env bash
# Shared command wiring for the opt-in Rust task file.
set -euo pipefail

require_tool() {
  command -v "$1" >/dev/null || {
    echo "Missing Rust tool: $1; declare it in your mise.toml [tools] and run mise install." >&2
    exit 1
  }
}

install_nightly() {
  : "${DEVTOOLS_RUST_FUZZ_TOOLCHAIN:?Set DEVTOOLS_RUST_FUZZ_TOOLCHAIN to a pinned nightly}"
  require_tool rustup
  rustup toolchain install "$DEVTOOLS_RUST_FUZZ_TOOLCHAIN" --profile minimal
}

measure_coverage() {
  require_tool cargo-llvm-cov
  require_tool cargo-nextest
  cargo llvm-cov nextest --workspace --locked --no-report
}

action=$1
shift
require_tool cargo
case "$action" in
build) cargo build --workspace --locked "$@" ;;
test)
  require_tool cargo-nextest
  cargo nextest run --workspace --locked "$@"
  cargo test --workspace --doc --locked
  ;;
clippy)
  require_tool cargo-clippy
  cargo clippy --workspace --all-targets --locked -- -D warnings "$@"
  ;;
rustdoc) RUSTDOCFLAGS="${RUSTDOCFLAGS:+$RUSTDOCFLAGS }-D warnings" cargo doc --workspace --no-deps --document-private-items --locked "$@" ;;
shear)
  require_tool cargo-shear
  cargo shear "$@"
  ;;
deny | vuln)
  require_tool cargo-deny
  if [[ "$action" == deny ]]; then
    cargo deny check advisories bans sources "$@"
  else
    cargo deny check advisories "$@"
  fi
  ;;
fmt | fmt-check)
  require_tool cargo-fmt
  if [[ "$action" == fmt-check ]]; then
    cargo fmt --all --check "$@"
  else
    cargo fmt --all "$@"
  fi
  ;;
coverage)
  : "${DEVTOOLS_RUST_COVER_LINES:?Set DEVTOOLS_RUST_COVER_LINES to your line coverage floor}"
  : "${DEVTOOLS_RUST_COVER_REGIONS:?Set DEVTOOLS_RUST_COVER_REGIONS to your region coverage floor}"
  measure_coverage
  cargo llvm-cov report --summary-only --fail-under-lines "$DEVTOOLS_RUST_COVER_LINES" --fail-under-regions "$DEVTOOLS_RUST_COVER_REGIONS" "$@"
  ;;
coverage-report)
  measure_coverage
  cargo llvm-cov report --html "$@"
  ;;
mutants)
  require_tool cargo-mutants
  require_tool cargo-nextest
  # Surviving mutants and timeouts are report results, not score gates.
  status=0
  cargo mutants --workspace --test-tool nextest "$@" || status=$?
  case "$status" in
  0) ;;
  2 | 3) test -f mutants.out/outcomes.json ;;
  *) exit "$status" ;;
  esac
  ;;
install-fuzz-toolchain) install_nightly ;;
fuzz | fuzz-smoke)
  require_tool cargo-fuzz
  install_nightly
  export RUSTUP_TOOLCHAIN="$DEVTOOLS_RUST_FUZZ_TOOLCHAIN"
  cd fuzz
  if [[ "$action" == fuzz ]]; then
    cargo fuzz run "$@"
  else
    targets=$(cargo fuzz list)
    while IFS= read -r target; do
      [[ -n "$target" ]] || continue
      cargo fuzz run "$target" -- -max_total_time="${DEVTOOLS_RUST_FUZZ_SECONDS:-20}"
    done <<<"$targets"
  fi
  ;;
esac
