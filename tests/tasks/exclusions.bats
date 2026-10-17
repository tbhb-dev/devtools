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

setup_tree_gates() {
	cp "$ROOT/.config/mise/conf.d/30-devtools-skill-contract.toml" "$WORK/.config/mise/conf.d/"
	grep '^uv = ' "$ROOT/mise.toml" >>mise.toml
	mkdir -p "$WORK/.vale/config/templates"
	cp "$ROOT/.vale/config/templates/ai-tells-agent.tmpl" "$WORK/.vale/config/templates/"
	cp "$ROOT/.devtools/tasks/go.toml" "$WORK/.devtools/tasks/"
	mkdir -p "$WORK/.devtools/scripts" "$WORK/.vale/Test"
	cp "$ROOT/.devtools/scripts/lint-go-arch.py" "$WORK/.devtools/scripts/"
	cat >>mise.toml <<TOML
[env]
DEVTOOLS_DIR = "$WORK/.devtools"
[tasks.lint-prose]
extends = "devtools:lint-prose"
TOML
	# Use the published Go task without pulling in the other Go gates.
	cat "$WORK/.devtools/tasks/go.toml" >>"$WORK/.devtools/tasks/lint.toml"
	mkdir -p cmd internal
	printf 'package cmd\n' >cmd/main.go
	printf 'package internal\n' >internal/main.go
	printf 'module example.com/test\n\ngo 1.26\n' >go.mod
	cp "$ROOT/.devtools/config/go/.go-arch-lint.yml" .go-arch-lint.yml
	printf 'StylesPath = .vale\n[*.md]\nBasedOnStyles = Test\n' >.vale.ini
	printf 'extends: existence\nmessage: Forbidden word\nlevel: error\ntokens: [forbidden]\n' >.vale/Test/Forbidden.yml
}

bad_tree_sources() {
	mkdir -p "$1"
	printf 'package bad\n' >"$1/bad.go"
	printf 'forbidden\n' >"$1/bad.md"
}

check_tree_gates() {
	for gate in lint-go-arch lint-prose; do
		if [[ "$gate" == lint-prose ]]; then
			run mise run lint-prose
		else
			run mise run "devtools:$gate"
		fi
		[ "$status" -eq "$1" ] || {
			echo "$gate: $output"
			return 1
		}
		if [[ "$1" -ne 0 ]]; then
			case "$gate" in
			lint-go-arch) [[ "$output" == *'not attached to any component'* ]] ;;
			lint-prose) [[ "$output" == *'Forbidden word'* ]] ;;
			esac
		fi
	done
}

@test "architecture and prose ignore tool installs but reject owned violations" {
	setup_tree_gates
	printf 'installed/\n' >.gitignore
	bad_tree_sources installed
	check_tree_gates 0
	bad_tree_sources src
	git add -- src
	check_tree_gates 1
}

@test "architecture and prose honor consumer generated paths" {
	setup_tree_gates
	bad_tree_sources 'generated output'
	git add -- 'generated output'
	export DEVTOOLS_GENERATED_PATHS='generated output/'
	check_tree_gates 0
	unset DEVTOOLS_GENERATED_PATHS
	check_tree_gates 1
}

@test "architecture exclusions survive a symlinked working directory" {
	setup_tree_gates
	printf 'installed/\n' >.gitignore
	bad_tree_sources installed
	ln -s "$BATS_TEST_TMPDIR" "$BATS_TEST_TMPDIR/linked-parent"
	cd "$BATS_TEST_TMPDIR/linked-parent/consumer"
	check_tree_gates 0
	bad_tree_sources src
	git add -- src
	check_tree_gates 1
}

@test "shared shell lint rejects masked returns and accepts checked substitutions" {
	printf '#!/usr/bin/env bash\n[[ $(false) == expected ]]\n' >check.sh
	run mise run devtools:lint-shell
	[ "$status" -ne 0 ]
	[[ "$output" == *SC2312* ]]
	printf '#!/usr/bin/env bash\nvalue=$(false) || exit\n[[ $value == expected ]]\n' >check.sh
	run mise run devtools:lint-shell
	[ "$status" -eq 0 ]
	# SC2155 must remain enabled alongside the optional rule.
	printf '#!/usr/bin/env bash\ncheck() { local value=$(false); echo "$value"; }; check\n' >check.sh
	run mise run devtools:lint-shell
	[ "$status" -ne 0 ]
	[[ "$output" == *SC2155* ]]
}

@test "curl task ships its checker and uses shared source exclusions" {
	mkdir -p .devtools/scripts src vendor generated
	cp "$ROOT/.devtools/scripts/check_curl_fail.py" .devtools/scripts/
	export DEVTOOLS_DIR="$WORK/.devtools"
	run mise run devtools:lint-curl
	[ "$status" -eq 0 ]
	printf '#!/bin/bash\ncurl url | cat\n' >'src/check curl.sh'
	cp 'src/check curl.sh' vendor/check.sh
	cp 'src/check curl.sh' generated/check.sh
	export DEVTOOLS_GENERATED_PATHS=generated
	run mise run devtools:lint-curl
	[ "$status" -ne 0 ]
	[[ "$output" == *'src/check curl.sh:2: curl without a fail option'* ]]
	[[ "$output" != *'vendor/check.sh:2:'* ]]
	[[ "$output" != *'generated/check.sh:2:'* ]]
	printf '#!/bin/bash\ncurl --fail url | cat\n' >'src/check curl.sh'
	run mise run devtools:lint-curl
	[ "$status" -eq 0 ]
}
