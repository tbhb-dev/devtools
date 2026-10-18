#!/usr/bin/env bats

setup() {
	source "${BATS_TEST_DIRNAME}/../test_helper.bash"
	isolate_git
	ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
	WORK="${BATS_TEST_TMPDIR}/consumer"
	mkdir -p "$WORK/.devtools" "$WORK/bin" "$WORK/fuzz"
	cp -R "$ROOT/.devtools/tasks" "$ROOT/.devtools/scripts" "$WORK/.devtools/"
	cp -R "$ROOT/tests/tasks/fixtures/rust/." "$WORK/"
	cd "$WORK" || return
	export DEVTOOLS_DIR="$WORK/.devtools" MISE_TRUSTED_CONFIG_PATHS="$WORK"
	export CALLS="$WORK/calls"
	cat >mise.toml <<'TOML'
[task_config]
includes = [".devtools/tasks/rust.toml"]
[env]
_.path = ["bin"]
TOML
	cat >bin/cargo <<'SH'
#!/usr/bin/env bash
set -eu
printf '%s|%s|%s\n' "$PWD" "${RUSTUP_TOOLCHAIN:-stable}" "$*" >> "$CALLS"
if [[ "$*" == 'fuzz list' ]]; then printf 'first\nsecond\n'; fi
if [[ "$*" == nextest* ]]; then exit "${NEXTEST_STATUS:-0}"; fi
if [[ "$*" == mutants* ]]; then
  if [[ "${OUTCOMES:-0}" == 1 ]]; then mkdir -p mutants.out; touch mutants.out/outcomes.json; fi
  exit "${MUTANTS_STATUS:-0}"
fi
SH
	chmod +x bin/cargo
	for tool in cargo-nextest cargo-clippy cargo-fmt cargo-shear cargo-deny cargo-llvm-cov cargo-mutants cargo-fuzz rustup; do
		cp bin/cargo "bin/$tool"
	done
	unset DEVTOOLS_RUST_COVER_LINES DEVTOOLS_RUST_COVER_REGIONS DEVTOOLS_RUST_FUZZ_TOOLCHAIN
}

@test "Rust task commands preserve workspace, lock and argument wiring" {
	for task in build-rust test-rust lint-rust-all format-rust vuln-rust cover-rust-report; do
		run mise run "devtools:$task"
		echo "$output"
		[ "$status" -eq 0 ]
	done
	for command in 'build --workspace --locked' 'nextest run --workspace --locked' 'test --workspace --doc --locked' 'fmt --all --check' 'clippy --workspace --all-targets --locked -- -D warnings' 'doc --workspace --no-deps --document-private-items --locked' 'shear' 'deny check advisories bans sources' 'deny check advisories' 'fmt --all' 'llvm-cov report --html'; do
		grep -Fq "|$command" "$CALLS"
	done
	run mise run devtools:test-rust -- -E 'test(example)'
	[ "$status" -eq 0 ]
	grep -Fq '|nextest run --workspace --locked -E test(example)' "$CALLS"
}

@test "nextest failure stops before doc tests and names missing tools" {
	run env NEXTEST_STATUS=7 mise run devtools:test-rust
	[ "$status" -ne 0 ]
	! grep -Fq '|test --workspace --doc' "$CALLS"
	rm bin/cargo-nextest
	# Exclude host mise shims so installed tools cannot mask this missing-tool case.
	run env PATH="$WORK/bin:/usr/bin:/bin" bash .devtools/scripts/rust.sh test
	[ "$status" -ne 0 ]
	[[ "$output" == *'Missing Rust tool: cargo-nextest'* ]]
}

@test "coverage requires consumer floors and separates measurement from reporting" {
	run mise run devtools:cover-rust
	[ "$status" -ne 0 ]
	[[ "$output" == *'DEVTOOLS_RUST_COVER_LINES'* ]]
	run env DEVTOOLS_RUST_COVER_LINES=91 DEVTOOLS_RUST_COVER_REGIONS=87 mise run devtools:cover-rust
	[ "$status" -eq 0 ]
	[ "$(wc -l <"$CALLS")" -eq 2 ]
	grep -Fq '|llvm-cov nextest --workspace --locked --no-report' "$CALLS"
	grep -Fq '|llvm-cov report --summary-only --fail-under-lines 91 --fail-under-regions 87' "$CALLS"
}

@test "mutation outcomes accept score results but fail tool errors" {
	run env MUTANTS_STATUS=2 mise run devtools:mutate-rust
	[ "$status" -ne 0 ]
	for code in 2 3; do
		run env MUTANTS_STATUS="$code" OUTCOMES=1 mise run devtools:mutate-rust
		[ "$status" -eq 0 ]
	done
	run env MUTANTS_STATUS=1 OUTCOMES=1 mise run devtools:mutate-rust
	[ "$status" -ne 0 ]
}

@test "fuzz tasks require a nightly and run targets from the fuzz directory" {
	run mise run devtools:fuzz-rust-smoke
	[ "$status" -ne 0 ]
	[[ "$output" == *'DEVTOOLS_RUST_FUZZ_TOOLCHAIN'* ]]
	export DEVTOOLS_RUST_FUZZ_TOOLCHAIN=nightly-2026-08-01
	run mise run devtools:fuzz-rust -- first -- -max_total_time=3
	echo "$output"
	[ "$status" -eq 0 ]
	grep -Fq "$WORK/fuzz|nightly-2026-08-01|fuzz run first -- -max_total_time=3" "$CALLS"
	run env DEVTOOLS_RUST_FUZZ_SECONDS=2 mise run devtools:fuzz-rust-smoke
	[ "$status" -eq 0 ]
	grep -Fq '|fuzz run second -- -max_total_time=2' "$CALLS"
}

@test "Rust templates parse and carry strict portable defaults" {
	run uv run --project "$ROOT" --no-sync python - "$ROOT" <<'PY'
import pathlib
import sys
import tomllib

source = pathlib.Path(sys.argv[1]) / '.devtools/config/rust'
configs = {p.name: tomllib.loads(p.read_text()) for p in source.glob('*.template')}
assert len(configs) == 5
assert configs['deny.toml.template']['advisories']['unmaintained'] == 'all'
assert configs['deny.toml.template']['bans']['multiple-versions'] == 'deny'
assert configs['clippy.toml.template']['msrv'] == '1.97.1'
assert configs['rustfmt.toml.template']['max_width'] == 120
profiles = configs['nextest.toml.template']['profile']
assert profiles['default']['retries'] == 0
assert profiles['default']['flaky-result'] == 'fail'
assert profiles['ci']['junit']['path'] == 'junit.xml'
lints = configs['workspace-lints.toml.template']['workspace']['lints']
assert lints['rust']['unsafe_code'] == 'forbid'
assert lints['clippy']['all']['level'] == 'deny'
assert lints['rustdoc']['broken_intra_doc_links'] == 'deny'
assert all('avirt' not in p.read_text() for p in source.glob('*.template'))
PY
	echo "$output"
	[ "$status" -eq 0 ]
}
