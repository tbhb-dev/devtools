#!/usr/bin/env bats

setup() {
	source "$BATS_TEST_DIRNAME/../../test_helper.bash"
	isolate_git
	ROOT=$(cd "$BATS_TEST_DIRNAME/../../.." && pwd)
	POLICY="$ROOT/packages/agents/claude/.apm/skills/merge-pr/scripts/merge-policy.py"
	mkdir -p "$BATS_TEST_TMPDIR/consumer"
	cd "$BATS_TEST_TMPDIR/consumer"
	git init --quiet
	git config user.name Tester
	git config user.email tester@example.com
	mkdir -p .agents .github/workflows
	cp -R "$ROOT/tests/tasks/fixtures/pins/." .
	cp "$ROOT/.devtools/vendir.yml" vendir.yml
	printf '\n"go:github.com/tbhb-dev/devtools/cmd/agentrepo" = "v0.10.0"\n' >>mise.toml
	cat >>apm.yml <<'YAML'
  - tbhb-dev/devtools/packages/agents/common#v0.10.0
  - 'tbhb-dev/devtools/packages/agents/agy#v0.10.0'
  - "tbhb-dev/devtools/packages/agents/claude#v0.10.0" # package pin
  - tbhb-dev/devtools/packages/agents/codex#v0.10.0
YAML
	cat >.github/workflows/ci.yaml <<'YAML'
jobs:
  lint:
    uses: tbhb-dev/devtools/.github/workflows/lint.yml@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa # v0.10.0
YAML
	cp .github/workflows/ci.yaml .github/workflows/ci.yml
	cat >.pre-commit-config.yaml <<'YAML'
repos:
  - repo: https://github.com/tbhb-dev/devtools
    rev: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  # frozen: v0.10.0
    hooks:
      - id: guard-markdown
YAML
	printf 'constitutional_paths = ^(apm[.]yml|mise[.]toml|[.]github/|vendir[.]yml|[.]pre-commit-config[.]yaml|protected[.]txt)\n' >.agents/agent-skills.conf
	printf 'baseline\n' >protected.txt
	git add -- .agents/agent-skills.conf apm.yml mise.toml .github vendir.yml .pre-commit-config.yaml protected.txt
	git commit --quiet -m 'test: consumer pin fixtures'
	BASE=$(git rev-parse HEAD)
}

bump() {
	local line
	while IFS= read -r line || [ -n "$line" ]; do
		line=${line//v0.10.0/v0.11.0}
		printf '%s\n' "${line//aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb}"
	done <"$1" >"$1.tmp"
	mv "$1.tmp" "$1"
}

inspect() {
	git add -- "$@"
	git commit --quiet -m 'test: edit pins'
	run uv run --quiet --script "$POLICY" inspect "$BASE" HEAD <<'JSON'
{"golden_paths":"^(apm[.]yml|mise[.]toml|[.]github/|vendir[.]yml|[.]pre-commit-config[.]yaml|protected[.]txt)","loosening_markers":"nolint","gate_config_paths":".*","owner_paths":"^mise[.]toml$","disposable_ignored":"^cache/"}
JSON
	[ "$status" -eq 0 ]
}

findings_are() {
	[[ "$output" == '{"findings": '"$1"', "owner_paths": '* ]]
}

pin_only() {
	bump "$1"
	inspect "$1"
	findings_are '[]'
}

@test "AC1 APM root and harness pin replacements have no finding" {
	pin_only apm.yml
}

@test "AC1 all managed mise tool pin replacements have no finding" {
	pin_only mise.toml
	[[ "$output" == *'"owner_paths": ["mise.toml"]}' ]]
}

@test "AC1 workflow pin replacements have no finding for either suffix" {
	bump .github/workflows/ci.yml
	bump .github/workflows/ci.yaml
	inspect .github/workflows/ci.yml .github/workflows/ci.yaml
	findings_are '[]'
}

@test "AC1 Renovate preset pin replacements have no finding" {
	pin_only .github/renovate.json5
}

@test "AC1 vendir pin replacements use the preceding devtools URL" {
	pin_only vendir.yml
}

@test "AC1 pre-commit pin replacements use the preceding devtools repository" {
	pin_only .pre-commit-config.yaml
}

@test "AC2 pin plus a non-pin line in the same protected file preserves all findings" {
	bump mise.toml
	printf 'go = "1.27.3"\n' >>mise.toml
	inspect mise.toml
	findings_are '[["mise.toml", "changes a constitutional file"], ["mise.toml", "changes a golden file"]]'
}

@test "AC2 pin plus another protected file preserves findings on both files" {
	bump mise.toml
	printf 'changed\n' >protected.txt
	inspect mise.toml protected.txt
	findings_are '[["mise.toml", "changes a constitutional file"], ["mise.toml", "changes a golden file"], ["protected.txt", "changes a constitutional file"], ["protected.txt", "changes a golden file"]]'
}

@test "AC2 content resembling a diff header remains a non-pin change" {
	bump apm.yml
	printf '++ b/non-pin\n' >>apm.yml
	inspect apm.yml
	findings_are '[["apm.yml", "changes a constitutional file"], ["apm.yml", "changes a golden file"]]'
}

@test "AC2 a pin plus a file mode change preserves findings" {
	bump mise.toml
	chmod +x mise.toml
	inspect mise.toml
	findings_are '[["mise.toml", "changes a constitutional file"], ["mise.toml", "changes a golden file"]]'
}

@test "AC3 unrecognized pin form preserves findings" {
	printf '[tools]\n"go:github.com/tbhb-dev/devtools/cmd/guard-markdown" = "main"\n' >mise.toml
	inspect mise.toml
	findings_are '[["mise.toml", "changes a constitutional file"], ["mise.toml", "changes a golden file"]]'
}

@test "AC3 adding a managed pinned tool preserves findings" {
	local line
	while IFS= read -r line; do
		[[ "$line" == *'/cmd/agentrepo"'* ]] || printf '%s\n' "$line"
	done <mise.toml >mise.toml.tmp
	mv mise.toml.tmp mise.toml
	git add -- mise.toml
	git commit --quiet -m 'test: omit optional pinned tool'
	BASE=$(git rev-parse HEAD)
	printf '"go:github.com/tbhb-dev/devtools/cmd/agentrepo" = "v0.11.0"\n' >>mise.toml
	inspect mise.toml
	findings_are '[["mise.toml", "changes a constitutional file"], ["mise.toml", "changes a golden file"]]'
}

@test "AC3 adding a pinned repository preserves findings" {
	printf '  - repo: https://github.com/tbhb-dev/devtools\n    rev: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  # frozen: v0.11.0\n' >>.pre-commit-config.yaml
	inspect .pre-commit-config.yaml
	findings_are '[[".pre-commit-config.yaml", "changes a constitutional file"], [".pre-commit-config.yaml", "changes a golden file"]]'
}

@test "AC3 adding a pin line preserves findings" {
	printf '  - tbhb-dev/devtools#v0.11.0\n' >>apm.yml
	inspect apm.yml
	findings_are '[["apm.yml", "changes a constitutional file"], ["apm.yml", "changes a golden file"]]'
}

@test "AC4 another repository never qualifies in any of the six pin forms" {
	local path line
	local paths=(apm.yml mise.toml .github/workflows/ci.yml .github/renovate.json5 vendir.yml .pre-commit-config.yaml)
	for path in "${paths[@]}"; do
		while IFS= read -r line; do
			printf '%s\n' "${line//tbhb-dev\/devtools/example\/devtools}"
		done <"$path" >"$path.tmp"
		mv "$path.tmp" "$path"
	done
	git add -- "${paths[@]}"
	git commit --quiet -m 'test: other repository pins'
	BASE=$(git rev-parse HEAD)
	for path in "${paths[@]}"; do
		bump "$path"
		inspect "$path"
		findings_are "[[\"$path\", \"changes a constitutional file\"], [\"$path\", \"changes a golden file\"]]" || return 1
		BASE=$(git rev-parse HEAD)
	done
}

@test "AC5 pin-looking suppression edits still produce the original findings" {
	bump .github/workflows/ci.yml
	printf '    uses: tbhb-dev/devtools/.github/workflows/test.yml@bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb # v0.11.0 nolint\n' >>.github/workflows/ci.yml
	inspect .github/workflows/ci.yml
	findings_are '[[".github/workflows/ci.yml", "changes a constitutional file"], [".github/workflows/ci.yml", "changes a golden file"], [".github/workflows/ci.yml", "adds a suppression or exclusion"]]'
}

extract_task() {
	local line selected=0 body=0
	while IFS= read -r line; do
		if [ "$line" = "[\"$1\"]" ]; then
			selected=1
		elif [ "$selected" -eq 1 ] && [[ "$line" == 'run = '* ]]; then
			body=1
		elif [ "$body" -eq 1 ]; then
			[ "$line" != "'''" ] || break
			printf '%s\n' "$line"
		fi
	done <"$ROOT/.devtools/tasks/pins.toml"
}

@test "AC6 actual update-pins rewrites every fixture and the reader accepts its diff" {
	extract_task devtools:update-pins >"$BATS_TEST_TMPDIR/update.sh"
	extract_task devtools:check-pins >"$BATS_TEST_TMPDIR/check.sh"
	local real_git
	real_git=$(command -v git)
	mkdir -p "$BATS_TEST_TMPDIR/bin"
	cat >"$BATS_TEST_TMPDIR/bin/git" <<STUB
#!/usr/bin/env bash
if [ "\$1" = ls-remote ]; then
  printf 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\trefs/tags/v0.11.0^{}\n'
else
  exec "$real_git" "\$@"
fi
STUB
	cat >"$BATS_TEST_TMPDIR/bin/mise" <<STUB
#!/usr/bin/env bash
case "\$*" in
'tasks ls --all') echo 'devtools:check-pins ' ;;
'run devtools:check-pins') exec bash "$BATS_TEST_TMPDIR/check.sh" ;;
esac
STUB
	printf '#!/usr/bin/env bash\nexit 0\n' >"$BATS_TEST_TMPDIR/bin/vendir"
	chmod +x "$BATS_TEST_TMPDIR/bin/"*
	run env PATH="$BATS_TEST_TMPDIR/bin:$PATH" usage_version=v0.11.0 bash "$BATS_TEST_TMPDIR/update.sh"
	[ "$status" -eq 0 ]
	[[ "$output" == *'every devtools pin is on v0.11.0'* ]]
	local paths
	paths=$(git diff --name-only)
	[ "$paths" = '.github/renovate.json5
.github/workflows/ci.yaml
.github/workflows/ci.yml
.pre-commit-config.yaml
apm.yml
mise.toml
vendir.yml' ]
	inspect apm.yml mise.toml .github vendir.yml .pre-commit-config.yaml
	findings_are '[]'
	[[ "$output" == *'"owner_paths": ["mise.toml"]}' ]]
}
