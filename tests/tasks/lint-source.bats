#!/usr/bin/env bats

setup_file() {
	local root
	root=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
	(cd "$root" && go build -o "$BATS_FILE_TMPDIR/agentrepo" ./cmd/agentrepo)
}

setup() {
	source "${BATS_TEST_DIRNAME}/../test_helper.bash"
	isolate_git
	ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
	export DEVTOOLS_DIR="$ROOT/.devtools"
	export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
	mkdir -p "$BATS_TEST_TMPDIR/bin" "$BATS_TEST_TMPDIR/repo"
	cd "$BATS_TEST_TMPDIR/repo"
	git init -q
	unset DEVTOOLS_GENERATED_PATHS DEVTOOLS_PROSE_GLOB
	export REAL_UV
	REAL_UV=$(command -v uv)
	cat >"$BATS_TEST_TMPDIR/bin/uv" <<'SH'
#!/usr/bin/env bash
if [[ $3 == */check_curl_fail.py ]]; then
 printf '%s\0' "$@" >"$RECORD"
 printf 'gate output\n'
 printf 'gate diagnostic\n' >&2
 exit "${GATE_STATUS:-0}"
fi
exec "$REAL_UV" "$@"
SH
	chmod +x "$BATS_TEST_TMPDIR/bin/uv"
	for gate in shellcheck shfmt vale guard-markdown; do
		cat >"$BATS_TEST_TMPDIR/bin/$gate" <<'SH'
#!/usr/bin/env bash
printf '%s\0' "$@" > "$RECORD"
printf 'gate output\n'
printf 'gate diagnostic\n' >&2
exit "${GATE_STATUS:-0}"
SH
		chmod +x "$BATS_TEST_TMPDIR/bin/$gate"
	done
}

compare() {
	export RECORD="$BATS_TEST_TMPDIR/frozen.args"
	local frozen_status=0 new_status=0
	bash "$ROOT/tests/tasks/fixtures/lint-source-files-frozen.sh" "$@" >frozen.out 2>frozen.err || frozen_status=$?
	export RECORD="$BATS_TEST_TMPDIR/new.args"
	"$BATS_FILE_TMPDIR/agentrepo" lint-source-files "$@" >new.out 2>new.err || new_status=$?
	[ "$frozen_status" -eq "$new_status" ]
	cmp frozen.out new.out
	cmp frozen.err new.err
	if [ -f "$BATS_TEST_TMPDIR/frozen.args" ]; then
		cmp "$BATS_TEST_TMPDIR/frozen.args" "$BATS_TEST_TMPDIR/new.args"
	else
		[ ! -e "$BATS_TEST_TMPDIR/new.args" ]
	fi
}

@test "owned source modes match frozen argv streams order and status" {
	mkdir -p src vendor apm_modules 'generated output' .agents/skills/deployed
	printf '#!/bin/bash\n' >'src/space name.sh'
	printf '#!/bin/bash\n' >$'src/line\nbreak.sh'
	printf 'prose\n' >src/doc.md
	cp 'src/space name.sh' vendor/skip.sh
	cp 'src/space name.sh' apm_modules/skip.sh
	cp 'src/space name.sh' 'generated output/skip.sh'
	cp 'src/space name.sh' .agents/skills/deployed/skip.sh
	cp 'src/space name.sh' .agents/skills/deployed/owned.sh
	printf 'ignored/\n' >.gitignore
	mkdir ignored
	cp src/doc.md ignored/doc.md
	git add -- src .agents vendor apm_modules
	printf 'untracked\n' >src/untracked.md
	cat >apm.lock.yaml <<'YAML'
dependencies:
- name: empty
- deployed_files:
  - .agents/skills/deployed
  - .agents/skills/deployed/skip.sh
YAML
	export DEVTOOLS_GENERATED_PATHS=$'generated output/\n\nsrc/doc.md\n'
	for mode in shell curl shell-fmt prose markdown-wrap; do
		export GATE_STATUS=17
		compare "$mode" --fix
		rm -f "$BATS_TEST_TMPDIR/frozen.args" "$BATS_TEST_TMPDIR/new.args"
	done
	export DEVTOOLS_PROSE_GLOB='!ignored/*'
	compare prose 'src/*' '.agents/*'
}

@test "empty selection and empty dependencies match frozen success without gate" {
	for lock in absent empty; do
		if [ "$lock" = empty ]; then printf 'dependencies: []\n' >apm.lock.yaml; fi
		for mode in shell curl shell-fmt prose markdown-wrap; do
			# Prose selects every file; narrow to an absent path.
			if [ "$mode" = prose ]; then compare "$mode" absent; else compare "$mode"; fi
		done
	done
}

@test "discovery and deployment errors never launch a gate" {
	cat >"$BATS_TEST_TMPDIR/bin/git" <<'SH'
#!/bin/bash
printf 'partial.sh\0'
printf 'discovery diagnostic\n' >&2
exit 23
SH
	chmod +x "$BATS_TEST_TMPDIR/bin/git"
	compare shell
	rm "$BATS_TEST_TMPDIR/bin/git"
	printf '#!/bin/bash\n' >check.sh
	cat >"$BATS_TEST_TMPDIR/bin/uv" <<'SH'
#!/bin/bash
printf 'filter diagnostic\n' >&2
exit 24
SH
	chmod +x "$BATS_TEST_TMPDIR/bin/uv"
	compare shell
}

@test "wrapper probes capable old absent unsupported and consumer-selected binaries" {
	uv run --project "$ROOT" --no-sync python - "$ROOT/.devtools/tasks/lint.toml" >task.sh <<'PY'
import sys, tomllib
with open(sys.argv[1], 'rb') as source:
    print(tomllib.load(source)['devtools:lint-source-files']['run'])
PY
	for kind in capable old unsupported absent; do
		rm -f "$BATS_TEST_TMPDIR/bin/agentrepo" "$BATS_TEST_TMPDIR/probed" "$BATS_TEST_TMPDIR/invoked"
		if [ "$kind" != absent ]; then
			cat >"$BATS_TEST_TMPDIR/bin/agentrepo" <<'SH'
#!/bin/bash
if [[ $* == 'lint-source-files --help' ]]; then
 touch "$BATS_TEST_TMPDIR/probed"
 case $KIND in
 capable) printf 'Usage: agentrepo lint-source-files MODE [ARG ...]\n';;
 unsupported) printf 'Usage: agentrepo [command]\n';;
 *) exit 1;;
 esac
else
 printf '%s\0' "$@" >"$BATS_TEST_TMPDIR/invoked"
 exit 27
fi
SH
			chmod +x "$BATS_TEST_TMPDIR/bin/agentrepo"
		fi
		export KIND=$kind
		if [ "$kind" = absent ]; then
			agentrepo() { return 127; }
			export -f agentrepo
		fi
		run bash task.sh shell --external-option
		unset -f agentrepo
		if [ "$kind" = capable ]; then
			[ "$status" -eq 27 ]
			printf '%s\0' lint-source-files shell --external-option >expected
			cmp expected "$BATS_TEST_TMPDIR/invoked"
		else
			[ "$status" -eq 0 ]
			[ ! -f "$BATS_TEST_TMPDIR/invoked" ]
		fi
	done
}
