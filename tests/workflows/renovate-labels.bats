#!/usr/bin/env bats

# Every pull request carries one `change:` label, and Renovate opens its own
# pull requests, so the shared preset has to supply `change:deps`.

setup() {
	preset="${BATS_TEST_DIRNAME}/../../.github/renovate-config.json5"
}

@test "preset labels every update change:deps" {
	run grep -c '^  labels: \["change:deps"\],$' "$preset"
	[ "$output" = 1 ]
}

# `labels` is not mergeable, so a consumer's own `labels` replaces it.
# `addLabels` is, and Renovate unions the two when it opens a pull request.
@test "preset keeps change:deps through a consumer labels override" {
	run grep -c '^  addLabels: \["change:deps"\],$' "$preset"
	[ "$output" = 1 ]
}

# Opt in because the ordinary Bats suite does not require Docker or an image pull.
# RUN_RENOVATE_LABELS=1 mise run test-hooks tests/workflows/renovate-labels.bats
@test "Renovate plans change:deps with and without a consumer labels setting" {
	[ "${RUN_RENOVATE_LABELS:-}" = 1 ] || skip 'set RUN_RENOVATE_LABELS=1 to run the pinned image'
	bats_require_minimum_version 1.5.0
	local root="${BATS_TEST_DIRNAME}/../.."
	local image
	image=$(sed -n 's/.*renovate-image: \([^ ]*\).*/\1/p' "$root/.github/workflows/renovate.yaml")
	[ -n "$image" ]
	run docker pull "$image"
	[ "$status" -eq 0 ]
	# The files travel on stdin, as in renovate-devtools.bats, because the
	# Docker daemon can't always see the host's paths for a bind mount.
	local work="${BATS_TEST_TMPDIR}/work"
	mkdir -p "$work"
	cp "$preset" "$work/preset.json5"
	cp "${BATS_TEST_DIRNAME}/fixtures/renovate-labels/plan.mjs" "$work/plan.mjs"
	tar --mtime=2020-01-01 -C "$work" -cf "$BATS_TEST_TMPDIR/work.tar" .
	plan() {
		docker run --pull=never --rm -i --network none -w /tmp -e LOG_LEVEL=fatal -e CONSUMER_LABELS="$1" \
			--entrypoint sh "$image" -c 'cd "$(mktemp -d)" && tar -xf - && node plan.mjs preset.json5' \
			<"$BATS_TEST_TMPDIR/work.tar"
	}
	run --separate-stderr plan ''
	[ "$status" -eq 0 ]
	[ "$output" = 'PLANNED_LABELS ["change:deps"]' ]
	run --separate-stderr plan '["deps"]'
	[ "$status" -eq 0 ]
	[ "$output" = 'PLANNED_LABELS ["change:deps","deps"]' ]
}
