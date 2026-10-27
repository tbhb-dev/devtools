#!/usr/bin/env bats
# cspell:words cksum

setup_file() {
	ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
	export ROOT
	go build -o "$BATS_FILE_TMPDIR/agentrepo" "$ROOT/cmd/agentrepo"
}

setup() {
	source "${BATS_TEST_DIRNAME}/../test_helper.bash"
	isolate_git
	ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
	BIN="$BATS_TEST_TMPDIR/bin"
	mkdir -p "$BIN" "$BATS_TEST_TMPDIR/repo"
	cd "$BATS_TEST_TMPDIR/repo"
	git init -q
	cp "$ROOT/sgconfig.yml" .
	cp -r "$ROOT/.ast-grep" .
	export PATH="$BIN:$PATH"
	SHIM="$ROOT/tools/check-script-hygiene.sh"
	FROZEN="$ROOT/tools/legacy/check-script-hygiene.sh"
}

stub() {
	printf '#!/bin/bash\n%s\n' "$2" >"$BIN/$1"
	chmod +x "$BIN/$1"
}

compare() {
	bash "$FROZEN" "$@" >"$BATS_TEST_TMPDIR/old.out" 2>"$BATS_TEST_TMPDIR/old.err" && old_status=0 || old_status=$?
	"$BATS_FILE_TMPDIR/agentrepo" check-script-hygiene "$@" >"$BATS_TEST_TMPDIR/new.out" 2>"$BATS_TEST_TMPDIR/new.err" && new_status=0 || new_status=$?
	[ "$old_status" -eq "$new_status" ]
	diff -u "$BATS_TEST_TMPDIR/old.out" "$BATS_TEST_TMPDIR/new.out"
	diff -u "$BATS_TEST_TMPDIR/old.err" "$BATS_TEST_TMPDIR/new.err"
}

@test "discovery and explicit corpus matches frozen streams and leaves state unchanged" {
	mkdir -p tools hooks scripts packages/agents/codex/.apm/skills/published/scripts .claude/skills/{published,local}/scripts
	printf 'git log HEAD\ngh pr view\ngitr diff "flag --no-ext-diff"\nIFS=$(printf '\'' \\t\\n'\'')\n' >tools/tracked.sh
	printf 'echo "git show"\nmy-git show\n# gh pr view\n' >scripts/new.sh
	printf 'export LC_ALL=anything\nunset GH_REPO\ngh pr view\ngitr diff --no-ext-diff\nIFS=$(printf '\''\\t'\'')\n' >hooks/clean.sh
	printf 'git show HEAD\n' >packages/agents/codex/.apm/skills/published/scripts/a.sh
	printf 'gh pr view\n' >.claude/skills/published/scripts/a.sh
	printf 'git status\n' >.claude/skills/local/scripts/a.sh
	printf '# hygiene-exempt: fixture reason\ngh pr view\ngit log HEAD\n' >tools/exempt.sh
	printf 'gh pr view\n' >tools/ignored.sh
	echo tools/ignored.sh >.gitignore
	git add -- tools/tracked.sh
	find . -type f -exec cksum {} + | sort >"$BATS_TEST_TMPDIR/files-before"
	git status --porcelain=v1 --untracked-files=all >"$BATS_TEST_TMPDIR/before"
	cp .git/index "$BATS_TEST_TMPDIR/index"
	git for-each-ref >"$BATS_TEST_TMPDIR/refs"
	compare
	[ "$new_status" -eq 1 ]
	compare missing tools/exempt.sh hooks/clean.sh
	[ "$new_status" -eq 0 ]
	[ ! -s "$BATS_TEST_TMPDIR/new.out" ]
	compare scripts/new.sh tools/tracked.sh scripts/new.sh
	git status --porcelain=v1 --untracked-files=all >"$BATS_TEST_TMPDIR/after"
	cmp "$BATS_TEST_TMPDIR/before" "$BATS_TEST_TMPDIR/after"
	cmp .git/index "$BATS_TEST_TMPDIR/index"
	git for-each-ref >"$BATS_TEST_TMPDIR/after-refs"
	cmp "$BATS_TEST_TMPDIR/refs" "$BATS_TEST_TMPDIR/after-refs"
	find . -type f -exec cksum {} + | sort >"$BATS_TEST_TMPDIR/files-after"
	cmp "$BATS_TEST_TMPDIR/files-before" "$BATS_TEST_TMPDIR/files-after"
}

@test "explicit paths preserve symlink followed by parent traversal" {
	mkdir -p nested/dir
	ln -s nested/dir alias
	printf '# hygiene-exempt: fixture reason\ngh pr view\n' >example.sh
	printf 'gh pr view\ngit log HEAD\n' >nested/example.sh
	compare alias/../example.sh
	[ "$new_status" -eq 1 ]
	[[ $(cat "$BATS_TEST_TMPDIR/new.out") == *missing-locale-pin* ]]
	[[ $(cat "$BATS_TEST_TMPDIR/new.out") == *missing-gh-unset* ]]
	[[ $(cat "$BATS_TEST_TMPDIR/new.out") == *bare-git-output* ]]
}

@test "discovery and checker failures preserve diagnostics and never report clean" {
	echo 'echo clean' >example.sh
	for selection in owned local; do
		export selection
		stub git 'if [[ $* == *rev-parse* ]]; then pwd; elif [[ $selection == owned || $* == *claude/skills* ]]; then echo example.sh; echo discovery-broke >&2; exit 23; fi'
		run "$BATS_FILE_TMPDIR/agentrepo" check-script-hygiene
		[ "$status" -eq 23 ]
		[[ $output == *discovery-broke* ]]
	done
	rm "$BIN/git"
	for exit_status in 2 23 127; do
		export exit_status
		stub ast-grep 'echo '\''{"file":"example.sh","range":{"start":{"line":0}},"ruleId":"r","message":"m"}'\''; echo checker-broke >&2; exit "$exit_status"'
		compare example.sh
		[ "$new_status" -eq 1 ]
	done
	stub ast-grep 'echo "{"'
	run "$BATS_FILE_TMPDIR/agentrepo" check-script-hygiene example.sh
	[ "$status" -eq 1 ]
	[[ $output == *invalid*diagnostics* ]]
}

@test "wrapper capability is read only and supported failures never fall back or repeat" {
	echo 'echo clean' >example.sh
	stub git 'echo mutation-trap >&2; exit 99'
	stub ast-grep 'echo mutation-trap >&2; exit 99'
	run "$BATS_FILE_TMPDIR/agentrepo" check-script-hygiene --probe
	[ "$status" -eq 0 ]
	[ "$output" = script-hygiene-v1 ]
	for failure in 1 23; do
		export failure
		stub agentrepo 'echo "$*" >>"$BATS_TEST_TMPDIR/calls"; if [[ $2 == --probe ]]; then echo script-hygiene-v1; else echo partial; exit "$failure"; fi'
		run bash "$SHIM" example.sh
		[ "$status" -eq "$failure" ]
		[ "$output" = partial ]
		[ "$(wc -l <"$BATS_TEST_TMPDIR/calls")" -eq 2 ]
		rm "$BATS_TEST_TMPDIR/calls"
	done
	stub agentrepo 'echo indeterminate; exit 23'
	run bash "$SHIM" example.sh
	[ "$status" -eq 1 ]
	[[ $output == *capability*probe*failed* ]]
	[[ $output != *mutation-trap* ]]
}

@test "absent and older binaries run frozen fallback with unchanged arguments" {
	echo 'gh pr view' >example.sh
	mkdir -p tools/legacy
	cp "$SHIM" tools/check-script-hygiene.sh
	cp "$FROZEN" tools/legacy/check-script-hygiene.sh
	task=$(uv run --project "$ROOT" --no-sync python -c 'import sys,tomllib; print(tomllib.load(open(sys.argv[1],"rb"))["tasks"]["lint-script-hygiene"]["run"])' "$ROOT/mise.toml")
	stub ast-grep 'echo "{}"'
	for mode in absent older supported; do
		case $mode in
		absent)
			mkdir -p "$BIN/absent"
			for dependency in bash git grep jq sort mktemp cat rm dirname; do
				ln -s "$(command -v "$dependency")" "$BIN/absent/$dependency"
			done
			cp "$BIN/ast-grep" "$BIN/absent/ast-grep"
			;;
		older) stub agentrepo 'echo '\''unknown command "check-script-hygiene" for "agentrepo"'\'' >&2; exit 5' ;;
		supported) cp "$BATS_FILE_TMPDIR/agentrepo" "$BIN/agentrepo" ;;
		esac
		bash "$FROZEN" missing example.sh >"$BATS_TEST_TMPDIR/expected" && expected_status=0 || expected_status=$?
		if [ "$mode" = absent ]; then
			run env PATH="$BIN/absent" /bin/bash "$SHIM" missing example.sh
		else
			run bash "$SHIM" missing example.sh
		fi
		[ "$status" -eq "$expected_status" ]
		[ "$output" = "$(cat "$BATS_TEST_TMPDIR/expected")" ]
		if [ "$mode" = absent ]; then
			run env PATH="$BIN/absent" /bin/bash -c "$task missing example.sh"
		else
			run bash -c "$task missing example.sh"
		fi
		[ "$status" -eq "$expected_status" ]
		[ "$output" = "$(cat "$BATS_TEST_TMPDIR/expected")" ]
	done
}
