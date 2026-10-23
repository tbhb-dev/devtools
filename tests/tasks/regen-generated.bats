#!/usr/bin/env bats

setup() {
	source "${BATS_TEST_DIRNAME}/../test_helper.bash"
	isolate_git
	ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
	WORK="${BATS_TEST_TMPDIR}/repo"
	mkdir -p "$WORK/bin" "$WORK/tools"
	awk '/^\[tasks.regen-generated\]$/ { selected = 1; print; next }
    selected && /^\[/ { exit }
    selected { print }' "$ROOT/mise.toml" >"$WORK/mise.toml"
	cat >>"$WORK/mise.toml" <<'TOML'
[env]
_.path = ["bin"]
[tasks.generate-skills]
run = "bash tools/generate-skills.sh"
[tasks.skill-tokens]
run = "bash tools/skill-tokens.sh"
TOML
	cat >"$WORK/tools/generate-skills.sh" <<'SH'
printf 'generate-skills\n' >>"$CALLS"
exit "${GENERATE_STATUS:-0}"
SH
	printf 'printf "skill-tokens\\n" >>"$CALLS"\nexit "${TOKENS_STATUS:-0}"\n' >"$WORK/tools/skill-tokens.sh"
	cat >"$WORK/bin/apm" <<'SH'
#!/bin/sh
printf 'apm %s\n' "$*" >>"$CALLS"
exit "${APM_STATUS:-0}"
SH
	chmod +x "$WORK/bin/apm"
	export CALLS="$WORK/calls" MISE_TRUSTED_CONFIG_PATHS="$WORK"
	cd "$WORK"
}

@test "AC1 regen-generated uses an ordered run array and propagates step failures" {
	run grep -Fx 'run = ["mise run generate-skills", "mise run skill-tokens", "apm install"]' mise.toml
	[ "$status" -eq 0 ]
	for failure in TOKENS_STATUS APM_STATUS; do
		run env "$failure=7" mise run regen-generated
		[ "$status" -ne 0 ]
	done
}

@test "AC2 regeneration docs name the aggregate command" {
	for path in AGENTS.md \
		packages/agents/source/skills/rebase/parts/verify.md \
		packages/agents/source/skills/resolve-rebase-conflicts/parts/take-side.md; do
		run grep -F 'mise run regen-generated' "$ROOT/$path"
		[ "$status" -eq 0 ]
		run grep -E 'mise run generate-skills.*mise run skill-tokens' "$ROOT/$path"
		[ "$status" -eq 1 ]
	done
}

@test "AC3 regen-generated runs in order and stops after a failed first step" {
	run mise run regen-generated
	[ "$status" -eq 0 ]
	printf '%s\n' generate-skills skill-tokens 'apm install' >expected
	diff -u expected "$CALLS"

	: >"$CALLS"
	run env GENERATE_STATUS=7 mise run regen-generated
	[ "$status" -ne 0 ]
	printf 'generate-skills\n' >expected
	diff -u expected "$CALLS"
}
