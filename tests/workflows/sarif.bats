#!/usr/bin/env bats

setup() {
	ROOT=$(cd "$BATS_TEST_DIRNAME/../.." && pwd)
	FILTER="$ROOT/.devtools/scripts/filter-govulncheck-sarif.sh"
}

@test "SARIF upload keeps reachable findings and preserves raw evidence and metadata" {
	raw="$BATS_TEST_DIRNAME/fixtures/govulncheck.sarif"
	cp "$raw" "$BATS_TEST_TMPDIR/raw.sarif"
	run bash "$FILTER" "$BATS_TEST_TMPDIR/raw.sarif" "$BATS_TEST_TMPDIR/upload.sarif"
	[ "$status" -eq 0 ]
	cmp "$raw" "$BATS_TEST_TMPDIR/raw.sarif"
	run jq -e '.runs[0].results | map(.ruleId) == ["reachable"]' "$BATS_TEST_TMPDIR/upload.sarif"
	[ "$status" -eq 0 ]
	run jq -e '.runs[1].results == [] and (.runs[2] | has("results") | not) and .version == "2.1.0" and .runs[0].tool.driver.name == "govulncheck"' "$BATS_TEST_TMPDIR/upload.sarif"
	[ "$status" -eq 0 ]
}

@test "malformed SARIF fails and identical paths cannot destroy raw evidence" {
	printf 'invalid\n' >"$BATS_TEST_TMPDIR/raw.sarif"
	run bash "$FILTER" "$BATS_TEST_TMPDIR/raw.sarif" "$BATS_TEST_TMPDIR/upload.sarif"
	[ "$status" -ne 0 ]
	run bash "$FILTER" "$BATS_TEST_TMPDIR/raw.sarif" "$BATS_TEST_TMPDIR/raw.sarif"
	[ "$status" -ne 0 ]
	[ "$(cat "$BATS_TEST_TMPDIR/raw.sarif")" = invalid ]
}
