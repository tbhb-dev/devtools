#!/usr/bin/env bats

setup_file() {
	local root
	root=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
	# Exercise this checkout without requiring a globally installed guard.
	(cd "$root" && go build -o "$BATS_FILE_TMPDIR/bin/guard-markdown" ./cmd/guard-markdown)
}

setup() {
	source "${BATS_TEST_DIRNAME}/../test_helper.bash"
	isolate_git
	ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
	export PATH="$BATS_FILE_TMPDIR/bin:$PATH"
	WORK="${BATS_TEST_TMPDIR}/consumer"
	mkdir -p "$WORK/.devtools/tasks" "$WORK/.config/mise/conf.d"
	cp "$ROOT/.config/mise/conf.d/10-devtools-tools.toml" "$WORK/.config/mise/conf.d/"
	cp "$ROOT/.devtools/tasks/lint.toml" "$WORK/.devtools/tasks/"
	cp "$ROOT/.config/mise/mise.lock" "$WORK/.config/mise/"
	cp "$ROOT/mise.lock" "$WORK/"
	cd "$WORK"
	git init -q
	cat >mise.toml <<'TOML'
[task_config]
includes = [".devtools/tasks/lint.toml"]
TOML
	printf '\n[tools]\n' >>mise.toml
	grep '^shfmt = ' "$ROOT/mise.toml" >>mise.toml
	export MISE_TRUSTED_CONFIG_PATHS="$WORK"
	unset DEVTOOLS_GENERATED_PATHS
}

bad_sources() {
	mkdir -p "$1"
	printf '#!/usr/bin/env bash\nif true;then echo $undefined;fi\n' >"$1/bad script.sh"
	printf 'first line\nsecond line\n' >"$1/bad.md"
}

check_gates() {
	for gate in shell shell-fmt markdown-wrap; do
		run mise run "devtools:lint-$gate"
		[ "$status" -eq "$1" ] || {
			echo "$gate: $output"
			return 1
		}
		if [[ "$1" -ne 0 ]]; then
			case "$gate" in
			shell) [[ "$output" == *SC2086* ]] || {
				echo "$output"
				return 1
			} ;;
			shell-fmt) [[ "$output" == *'@@ '* ]] || {
				echo "$output"
				return 1
			} ;;
			markdown-wrap) [[ "$output" == *'error: guard-markdown:'* ]] ;;
			esac
		fi
	done
}

@test "shared gates skip empty repositories and default excluded trees" {
	check_gates 0
	for directory in vendor apm_modules .claude .agents .codex; do
		bad_sources "$directory"
	done
	git add -f -- vendor apm_modules .claude .agents .codex
	check_gates 0
}

@test "generated paths extend defaults for directories and individual files" {
	bad_sources 'generated output'
	bad_sources custom
	bad_sources vendor
	export DEVTOOLS_GENERATED_PATHS=$'generated output/\ncustom/bad script.sh\ncustom/bad.md'
	check_gates 0
	unset DEVTOOLS_GENERATED_PATHS
	check_gates 1
}

@test "real tracked and untracked source violations fail every gate" {
	bad_sources src
	check_gates 1
	git add -- src
	check_gates 1
}

@test "architecture template excludes deployed and third-party directories" {
	for directory in vendor apm_modules .claude .agents .codex; do
		run grep -F -- "  - $directory" "$ROOT/.devtools/config/go/.go-arch-lint.yml"
		[ "$status" -eq 0 ]
	done
}

@test "Markdown fix arguments reach the selected files" {
	bad_sources src
	run mise run devtools:lint-markdown-wrap -- --fix
	[ "$status" -eq 0 ]
	run mise run devtools:lint-markdown-wrap
	[ "$status" -eq 0 ]
	[ "$(cat src/bad.md)" = 'first line second line' ]
}
