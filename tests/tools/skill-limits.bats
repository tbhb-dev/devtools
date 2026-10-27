#!/usr/bin/env bats

setup() {
	source "${BATS_TEST_DIRNAME}/../test_helper.bash"
	isolate_git
	SOURCE=$(cd "$BATS_TEST_DIRNAME/../.." && pwd)
	SCRIPT="$SOURCE/tools/skill-limits.sh"
	REPO="$BATS_TEST_TMPDIR/repo"
	BIN="$BATS_TEST_TMPDIR/bin"
	mkdir -p "$REPO/valid" "$REPO/bad" "$BIN"
	cd "$REPO"
	git init --quiet
	printf -- '---\nname: valid\ndescription: text\n---\n' >valid/SKILL.md
	printf 'body\n' >bad/SKILL.md
	export CALLS="$BATS_TEST_TMPDIR/calls"
	export TRAPS="$BATS_TEST_TMPDIR/traps"
	local tool
	for tool in curl wget ssh gh; do
		printf '#!/bin/sh\necho network >>"$TRAPS"\nexit 99\n' >"$BIN/$tool"
		chmod +x "$BIN/$tool"
	done
	export PATH="$BIN:$PATH"
}

stub() {
	cat >"$BIN/agentrepo"
	chmod +x "$BIN/agentrepo"
}

@test "older capability probe falls back once and preserves targets and transcript" {
	stub <<'SH'
#!/bin/sh
printf '%s\n' "$*" >>"$CALLS"
[ "$*" = --help ] || exit 99
printf 'Usage:\n  agentrepo\nAvailable Commands:\n  version\n'
SH
	run bash "$SCRIPT" bad/ missing valid
	[ "$status" -eq 1 ]
	[[ "$output" == *'bad/SKILL.md:1 [error] name-missing'* ]]
	[[ "$output" == *'TOTAL: 2 finding(s)'* ]]
	[ "$(cat "$CALLS")" = --help ]
	[ ! -e "$TRAPS" ]
}

@test "supported invocation failure preserves output and never falls back or repeats" {
	stub <<'SH'
#!/bin/sh
printf '%s\n' "$*" >>"$CALLS"
if [ "$*" = --help ]; then
  printf '  check-skill-limits  Check offline skill manifest limits\n'
  exit 0
fi
printf 'supported finding\n'
exit 42
SH
	run bash "$SCRIPT" 'bad/' missing
	[ "$status" -eq 42 ]
	[ "$output" = 'supported finding' ]
	[ "$(cat "$CALLS")" = $'--help\ncheck-skill-limits -- bad/ missing' ]
	[ ! -e "$TRAPS" ]
}

@test "failed probe refuses without fallback" {
	stub <<'SH'
#!/bin/sh
printf '%s\n' "$*" >>"$CALLS"
printf 'probe failed\n' >&2
exit 43
SH
	run bash "$SCRIPT" valid
	[ "$status" -eq 43 ]
	[ "$output" = 'probe failed' ]
	[ "$(cat "$CALLS")" = --help ]
	[ ! -e "$TRAPS" ]
}

@test "missing binary uses the frozen shell with unchanged arguments" {
	# Mask the installed tool without removing the system tools the fallback uses.
	local tool
	for tool in bash dirname git python3; do
		ln -s "$(command -v "$tool")" "$BIN/$tool"
	done
	run env PATH="$BIN" bash "$SCRIPT" missing valid/
	[ "$status" -eq 0 ]
	[ -z "$output" ]
	run env PATH="$BIN" bash "$SCRIPT" bad
	[ "$status" -eq 1 ]
	[[ "$output" == *'TOTAL: 2 finding(s)'* ]]
	[ ! -e "$TRAPS" ]
}

@test "failed parser cannot reuse valid fields even when it prints plausible values" {
	export REAL_PYTHON
	REAL_PYTHON=$(command -v python3)
	cat >"$BIN/python3" <<'SH'
#!/bin/sh
if [ "$2" = bad/SKILL.md ]; then
  printf "skill_name=valid\nskill_desc=text\n"
  exit 44
fi
exec "$REAL_PYTHON" "$@"
SH
	chmod +x "$BIN/python3"
	run bash "$SOURCE/tools/skill-limits-legacy.sh" valid bad
	[ "$status" -eq 1 ]
	[ "$output" = $'bad/SKILL.md:1 [error] unreadable  could not parse the frontmatter\nTOTAL: 1 finding(s)' ]
}

@test "new binary and unchanged mise task preserve frozen verdicts" {
	local executable="$BATS_TEST_TMPDIR/agentrepo-new"
	(cd "$SOURCE" && go build -o "$executable" ./cmd/agentrepo)
	ln -s "$executable" "$BIN/agentrepo"
	awk '
    /^\[/ { copy = ($0 == "[tasks.lint-skills]") }
    copy { print }
  ' "$SOURCE/mise.toml" >mise.toml
	mkdir tools
	cp "$SCRIPT" "$SOURCE/tools/skill-limits-legacy.sh" tools/
	export MISE_TRUSTED_CONFIG_PATHS="$REPO"
	mkdir -p packages/agents/codex/.apm/skills/bad
	cp bad/SKILL.md packages/agents/codex/.apm/skills/bad/SKILL.md

	local task_output real_mise tool
	real_mise=$(command -v mise)
	run mise --quiet run lint-skills
	[ "$status" -ne 0 ]
	task_output=$output
	[[ "$output" == *'packages/agents/codex/.apm/skills/bad/SKILL.md:1 [error] name-missing'* ]]
	[[ "$output" == *'TOTAL: 2 finding(s)'* ]]
	rm "$BIN/agentrepo"
	for tool in sh bash dirname git python3 grep; do
		ln -s "$(command -v "$tool")" "$BIN/$tool"
	done
	run env PATH="$BIN" "$real_mise" --quiet run lint-skills
	[ "$status" -ne 0 ]
	[ "$output" = "$task_output" ]
	stub <<'SH'
#!/bin/sh
[ "$*" = --help ] || exit 99
printf '  version  Print build version\n'
SH
	run env PATH="$BIN" "$real_mise" --quiet run lint-skills
	[ "$status" -ne 0 ]
	[ "$output" = "$task_output" ]
	rm "$BIN/agentrepo"
	ln -s "$executable" "$BIN/agentrepo"
	run bash "$SCRIPT" missing valid/
	[ "$status" -eq 0 ]
	[ -z "$output" ]
	run bash "$SCRIPT" bad/
	[ "$status" -eq 1 ]
	[ "$output" = $'bad/SKILL.md:1 [error] name-missing  the frontmatter sets no name\nbad/SKILL.md:1 [error] description-missing  the frontmatter sets no description\nTOTAL: 2 finding(s)' ]
	[ ! -e "$TRAPS" ]
}
