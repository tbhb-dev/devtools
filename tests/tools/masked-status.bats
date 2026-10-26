#!/usr/bin/env bats

setup() {
	source "${BATS_TEST_DIRNAME}/../test_helper.bash"
	isolate_git
	ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
	BIN="$BATS_TEST_TMPDIR/bin"
	mkdir -p "$BIN"
	export PATH="$BIN:$PATH"
	cd "$BATS_TEST_TMPDIR"
}

# Extract task bodies without launching mise or installing its toolchain.
task_body() {
	uv run --project "$ROOT" --no-sync python - "$ROOT/$1" "$2" <<'PY'
import sys, tomllib
with open(sys.argv[1], 'rb') as source:
    data = tomllib.load(source)
body = data.get('tasks', data)[sys.argv[2]]['run']
print('set -e\n' + '\n'.join(body) if isinstance(body, list) else body)
PY
}

stub() {
	printf '#!/bin/bash\n%s\n' "$2" >"$BIN/$1"
	chmod +x "$BIN/$1"
}

@test "hook inspection errors preserve files and prevent install in script and task" {
	stub agentrepo 'exit 1'
	stub mise 'exit 1'
	mkdir -p .devtools/scripts
	cp "$ROOT/.devtools/scripts/apm-sync.sh" .devtools/scripts/
	stub jq 'if [[ $1 == length ]]; then echo 0; exit 23; fi; echo "{}"'
	stub apm 'touch installed'
	task_body mise.toml apm-sync >task.sh
	for script in "$ROOT/.devtools/scripts/apm-sync.sh" "$PWD/task.sh"; do
		for file in .claude/settings.json .agents/hooks.json .codex/hooks.json; do
			rm -rf .claude .agents .codex
			mkdir -p "${file%/*}"
			echo '{}' >"$file"
			run bash "$script"
			[ "$status" -eq 23 ]
			[ -f "$file" ]
			[ ! -e installed ]
		done
	done
}

@test "build tasks stop when metadata prints flags and then fails" {
	mkdir -p tools payload/scripts
	printf 'echo flags; exit 23\n' >tools/go-ldflags.sh
	cp tools/go-ldflags.sh payload/scripts/go-ldflags.sh
	export DEVTOOLS_DIR="$PWD/payload"
	stub go 'touch invoked'
	for spec in 'mise.toml run' '.devtools/tasks/go.toml devtools:build' '.devtools/tasks/go.toml devtools:install'; do
		read -r file name <<<"$spec"
		task_body "$file" "$name" >task.sh
		run bash task.sh example
		[ "$status" -eq 23 ]
		[ ! -e invoked ]
	done
}

@test "coverage merge stops when listing prints a directory and fails" {
	stub ls 'echo coverage/; exit 23'
	stub go 'touch invoked'
	export usage_slotsdir=coverage
	task_body .devtools/tasks/go.toml devtools:cover-go-merge >task.sh
	run bash task.sh
	[ "$status" -eq 23 ]
	[ ! -e invoked ]
}

@test "changelog tasks propagate producer errors" {
	stub cog 'echo "- entry"; exit 23'
	stub rumdl 'cat'
	for name in generate-changelog generate-release-notes; do
		task_body mise.toml "$name" >task.sh
		run bash task.sh 1.0.0
		[ "$status" -eq 23 ]
	done
}

@test "release notes stop when resolving HEAD prints a hash and fails" {
	stub git 'echo abc; exit 23'
	stub cog 'touch invoked'
	task_body mise.toml generate-release-notes >task.sh
	run bash task.sh
	[ "$status" -eq 23 ]
	[ ! -e invoked ]
}

@test "source selection cannot pass on a failed git listing" {
	stub git 'printf "example.sh\0"; exit 23'
	stub shellcheck 'touch invoked'
	task_body .devtools/tasks/lint.toml devtools:lint-source-files >task.sh
	run bash task.sh shell
	[ "$status" -eq 23 ]
	[ ! -e invoked ]
}

@test "hygiene checks propagate both file discovery errors and diagnostic parser errors" {
	stub git 'if [[ $* == *rev-parse* ]]; then pwd; else echo tools/example.sh; exit 23; fi'
	run bash "$ROOT/tools/check-script-hygiene.sh"
	[ "$status" -eq 23 ]
	stub git 'if [[ $* == *rev-parse* ]]; then pwd; elif [[ $* == *claude/skills* ]]; then echo example.sh; exit 23; fi'
	run bash "$ROOT/tools/check-script-hygiene.sh"
	[ "$status" -eq 23 ]
	printf '#!/bin/bash\necho example\n' >example.sh
	stub ast-grep 'echo "{}"'
	stub jq 'exit 23'
	run bash "$ROOT/tools/check-script-hygiene.sh" example.sh
	[ "$status" -eq 23 ]
}

# Isolate the installer functions so probes never download or install mise.
bootstrap_function() {
	uv run --project "$ROOT" --no-sync python - "$ROOT/tools/mise-bootstrap" "$1" <<'PY'
import sys
text = open(sys.argv[1]).read()
start = text.index('        ' + sys.argv[2] + '() {')
end = text.index('\n        }', start) + len('\n        }')
print('\n'.join(line[8:] for line in text[start:end].split('\n')))
PY
}

@test "bootstrap rejects failed OS and architecture probes even after valid output" {
	for function in get_os get_arch; do
		bootstrap_function "$function" >probe.sh
		printf '\n%s\n' "$function" >>probe.sh
		stub uname 'case $1 in -s) echo Linux;; -o) echo Android;; -m) echo x86_64;; esac; exit 23'
		run bash probe.sh
		[ "$status" -eq 23 ]
	done
	stub uname 'case $1 in -o) echo GNU/Linux;; -m) echo x86_64;; esac'
	stub ldd 'echo glibc; exit 23'
	run bash probe.sh
	[ "$status" -eq 23 ]
}

@test "bootstrap does not trust a version printed by a failing binary" {
	stub broken 'echo 2026.8.0; exit 23'
	bootstrap_function installed_mise_version >probe.sh
	printf '\ninstalled_mise_version "$1"\n' >>probe.sh
	run bash probe.sh "$BIN/broken"
	[ "$status" -eq 23 ]
	[ -z "$output" ]
}

@test "pin checks reject failed count commands even when they print one" {
	task_body .devtools/tasks/pins.toml devtools:check-pins >task.sh
	# Feed the comparison section directly; discovery is covered by the pin suite.
	sed -n '/^version_count=/,$p' task.sh >comparison.sh
	stub wc 'echo 1; exit 23'
	run env versions=v1.0.0 shas=abc bash -e -o pipefail comparison.sh
	[ "$status" -eq 23 ]
}

@test "preapproval probes stop on failed platform and repository commands" {
	task_body .devtools/tasks/preapproval.toml devtools:preapprove >task.sh
	# Run only the probes: no test grants a pre-approval or invokes Touch ID.
	sed -n '/^  os=/,/^  if /p' task.sh | head -n 1 >probe.sh
	stub uname 'echo Darwin; exit 23'
	run bash -e probe.sh
	[ "$status" -eq 23 ]
	sed -n '/^  repository=/p' task.sh >probe.sh
	stub git 'echo repo; exit 23'
	run bash -e probe.sh
	[ "$status" -eq 23 ]
}

@test "bootstrap does not accept tar capability output from a failed tar" {
	bootstrap_function tar_supports_zstd >probe.sh
	printf '\ntar_supports_zstd\n' >>probe.sh
	stub zstd ':'
	stub tar 'echo bsdtar; exit 23'
	run bash probe.sh
	[ "$status" -eq 23 ]
}

@test "bootstrap stops before checksum validation when its command cannot be resolved" {
	bootstrap_function install_mise >probe.sh
	cat >>probe.sh <<'SH'
get_os() { echo linux; }
get_arch() { echo x64; }
get_ext() { echo tar.gz; }
download_file() { echo "$PWD/archive"; }
debug() { :; }
get_checksum() { touch validated; }
shasum_bin() { echo checksum; return 23; }
install_mise
SH
	export MISE_INSTALL_PATH="$PWD/mise"
	run bash -e -o pipefail probe.sh
	[ "$status" -eq 23 ]
	[ ! -e validated ]
}

@test "bootstrap stops on failed cache path and user ID probes" {
  bootstrap_function install_mise >probe.sh
  cat >>probe.sh <<'SH'
get_os() { echo linux; }
get_arch() { echo x64; }
get_ext() { echo tar.gz; }
download_file() { echo "$PWD/archive"; }
debug() { :; }
info() { :; }
get_checksum() { printf '%65536s\n' ignored; }
shasum_bin() { echo checksum; }
tar() { mkdir -p mise/bin; touch mise/bin/mise; }
install_mise
SH
  # A checksum verifier must drain its input before it exits. Keep the
  # producer larger than a pipe buffer to expose an early-exiting stub.
  stub checksum 'cat >/dev/null'
  real_dirname=$(command -v dirname)
  stub dirname "if [[ \$1 == */archive ]]; then echo \"\$PWD\"; exit 23; fi; exec '$real_dirname' \"\$@\""
  export MISE_INSTALL_PATH="$PWD/installed-mise"
  run bash -e -o pipefail probe.sh
  [ "$status" -eq 23 ]
  [ ! -e "$MISE_INSTALL_PATH" ]
  rm "$BIN/dirname"
  stub id 'echo 1000; exit 23'
  run bash -e -o pipefail probe.sh
  [ "$status" -eq 23 ]
  [ ! -e "$MISE_INSTALL_PATH" ]
}

@test "release clone stops when Git directory discovery fails" {
	stub git 'case $* in *--show-toplevel*) pwd ;; *--git-common-dir*) echo .git; exit 23 ;; esac'
	run bash "$ROOT/.claude/skills/release/scripts/release-clone.sh" path
	[ "$status" -eq 1 ]
	[[ "$output" == *'could not resolve the common Git directory'* ]]
	stub git 'case $* in *--show-toplevel*) pwd ;; *--git-common-dir*) echo .git ;; *--git-dir*) echo .git; exit 23 ;; esac'
	run bash "$ROOT/.claude/skills/release/scripts/release-clone.sh" path
	[ "$status" -eq 1 ]
	[[ "$output" == *'could not resolve the worktree Git directory'* ]]
}

@test "release version context propagates failed Git readers" {
	mkdir -p clone/.git
	stub cog 'echo 1.0.1'
	for failing in count reverse short subject; do
		export FAIL_READER=$failing
		stub git 'case $* in
  "describe "*) echo v1.0.0 ;;
  "rev-parse "*) echo abc ;;
  "rev-list --count "*) echo 1; [[ $FAIL_READER != count ]] || exit 23 ;;
  "rev-list --reverse "*) echo abc; [[ $FAIL_READER != reverse ]] || exit 23 ;;
  "log -1 --format=%h "*) echo abc; [[ $FAIL_READER != short ]] || exit 23 ;;
  "log -1 --format=%s "*) echo subject; [[ $FAIL_READER != subject ]] || exit 23 ;;
esac'
		run bash "$ROOT/.claude/skills/review-release-version/scripts/context.sh" "$PWD/clone"
		[ "$status" -eq 23 ]
	done
}
