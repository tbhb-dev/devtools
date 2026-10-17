#!/usr/bin/env bats

setup() {
	ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
	WORK="${BATS_TEST_TMPDIR}/consumer"
	mkdir -p "$WORK/.devtools/tasks" "$WORK/.config/mise/conf.d"
	cp "$ROOT/.config/mise/conf.d/10-devtools-tools.toml" "$WORK/.config/mise/conf.d/"
	cp "$ROOT/.devtools/tasks/lint.toml" "$WORK/.devtools/tasks/"
	cat >"$WORK/mise.toml" <<'TOML'
[task_config]
includes = [".devtools/tasks/lint.toml"]
TOML
	cd "$WORK"
	git init -q
	export MISE_TRUSTED_CONFIG_PATHS="$WORK"
}

@test "workflow lint skips missing and empty workflow directories" {
	run mise run devtools:lint-workflows
	[ "$status" -eq 0 ]
	[[ "$output" == *'No workflow files found; skipping actionlint.'* ]]
	mkdir -p .github/workflows
	run mise run devtools:lint-workflows
	[ "$status" -eq 0 ]
	[[ "$output" == *'No workflow files found; skipping actionlint.'* ]]
	printf 'not a workflow\n' >.github/workflows/README.txt
	run mise run devtools:lint-workflows
	[ "$status" -eq 0 ]
	[[ "$output" == *'No workflow files found; skipping actionlint.'* ]]
}

@test "workflow lint accepts valid YAML workflows and fails on findings" {
	mkdir -p .github/workflows
	for extension in yml yaml; do
		cat >".github/workflows/check.$extension" <<'YAML'
name: Check
on: push
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
YAML
		run mise run devtools:lint-workflows
		[ "$status" -eq 0 ]
		[[ "$output" != *'skipping actionlint'* ]]
		printf '      - run: echo ${{ nonexistent.value }}\n' >>".github/workflows/check.$extension"
		run mise run devtools:lint-workflows
		[ "$status" -ne 0 ]
		[[ "$output" == *'nonexistent'* ]]
		rm ".github/workflows/check.$extension"
	done
}
