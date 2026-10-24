#!/usr/bin/env bats

setup() {
	ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
	load ../test_helper
	isolate_git
	WORK="$BATS_TEST_TMPDIR/repo"
	mkdir -p "$WORK/tests" "$WORK/bin"
	git -C "$WORK" init -q
	touch "$WORK/tests/one.bats" "$WORK/tests/two.bats"
	printf '1 1.000 tests/one.bats\n2 2.000 tests/two.bats\n' >"$WORK/tests/bats-shards.tsv"
	cat >"$WORK/bin/bats" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$@"
STUB
	chmod +x "$WORK/bin/bats"
	export PATH="$WORK/bin:$PATH"
	cd "$WORK"
}

@test "partition accepts complete coverage and runs only the selected files" {
	run bash "$ROOT/tools/test-bats-shard.sh" --check
	[ "$status" -eq 0 ]
	run bash "$ROOT/tools/test-bats-shard.sh" 2
	[ "$status" -eq 0 ]
	[ "$output" = $'--timing\ntests/two.bats' ]
}

@test "partition rejects a missing file, including a newly added suite" {
	touch tests/new.bats
	run bash "$ROOT/tools/test-bats-shard.sh" 1
	[ "$status" -ne 0 ]
	[[ "$output" == *tests/new.bats* ]]
}

@test "partition rejects duplicate assignments and stale paths" {
	printf '3 1.000 tests/one.bats\n' >>tests/bats-shards.tsv
	run bash "$ROOT/tools/test-bats-shard.sh" --check
	[ "$status" -ne 0 ]
	[[ "$output" == *tests/one.bats* ]]
	printf '4 1.000 tests/gone.bats\n' >>tests/bats-shards.tsv
	run bash "$ROOT/tools/test-bats-shard.sh" --check
	[ "$status" -ne 0 ]
	[[ "$output" == *tests/gone.bats* ]]
}

@test "partition rejects invalid shard identifiers and malformed rows" {
	run bash "$ROOT/tools/test-bats-shard.sh" 5
	[ "$status" -ne 0 ]
	printf '5 1.000 tests/one.bats\n' >>tests/bats-shards.tsv
	run bash "$ROOT/tools/test-bats-shard.sh" --check
	[ "$status" -ne 0 ]
	[[ "$output" == *'invalid shard manifest row'* ]]
}
