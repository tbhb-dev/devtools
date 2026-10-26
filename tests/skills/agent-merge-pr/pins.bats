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
	printf '\n"go:github.com/tbhb-dev/devtools/cmd/agentrepo" = "v0.10.0" # tool pin\n' >>mise.toml
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
  lint-extra:
    uses: tbhb-dev/devtools/.github/workflows/lint.yml@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa # v0.10.0 extra
YAML
	cp .github/workflows/ci.yaml .github/workflows/ci.yml
	local line
	while IFS= read -r line; do
		if [[ "$line" == *'renovate-go-config.json5'* ]]; then
			line=${line/v0.10.0/v0.10.0 extra}
		fi
		printf '%s\n' "$line"
	done <.github/renovate.json5 >renovate.tmp
	mv renovate.tmp .github/renovate.json5
	cat >.pre-commit-config.yaml <<'YAML'
repos:
  - repo: https://github.com/tbhb-dev/devtools
    rev: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  # frozen: v0.10.0
    hooks:
      - id: guard-markdown
YAML
	printf '# adapted from tbhb-dev/devtools v0.10.0.\n' >tombi.toml
	printf 'constitutional_paths = ^(apm[.]yml|mise[.]toml|tombi[.]toml|[.]github/|vendir[.]yml|[.]pre-commit-config[.]yaml|protected[.]txt)\n' >.agents/agent-skills.conf
	printf 'baseline\n' >protected.txt
	git add -- .agents/agent-skills.conf apm.yml mise.toml .github vendir.yml .pre-commit-config.yaml tombi.toml protected.txt
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
{"golden_paths":"^(apm[.]yml|mise[.]toml|tombi[.]toml|[.]github/|vendir[.]yml|[.]pre-commit-config[.]yaml|protected[.]txt)","loosening_markers":"nolint","gate_config_paths":".*","owner_paths":"^mise[.]toml$","disposable_ignored":"^cache/"}
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

@test "AC2 changed workflow comment suffix preserves findings" {
	changed_suffix .github/workflows/ci.yml
}

@test "AC2 changed Renovate comment suffix preserves findings" {
	changed_suffix .github/renovate.json5
}

changed_suffix() {
	bump "$1"
	local line
	while IFS= read -r line; do
		printf '%s\n' "${line/ extra/ changed}"
	done <"$1" >"$1.tmp"
	mv "$1.tmp" "$1"
	inspect "$1"
	findings_are "[[\"$1\", \"changes a constitutional file\"], [\"$1\", \"changes a golden file\"]]"
}

@test "AC1 vendir pin replacements use the preceding devtools URL" {
	pin_only vendir.yml
}

@test "AC1 pre-commit pin replacements use the preceding devtools repository" {
	pin_only .pre-commit-config.yaml
}

@test "AC2 embedded CR preserves protected findings" {
	separator_attack '\r'
}

@test "AC2 embedded NEL preserves protected findings" {
	separator_attack '\302\205'
}

@test "AC2 embedded Unicode line separator preserves protected findings" {
	separator_attack '\342\200\250'
}

@test "AC2 embedded Unicode paragraph separator preserves protected findings" {
	separator_attack '\342\200\251'
}

separator_attack() {
	bump .github/workflows/ci.yml
	printf '    uses: tbhb-dev/devtools/.github/workflows/lint.yml@bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb # v0.11.0%b    secrets: inherit\n' "$1" >.github/workflows/ci.yml.tmp
	# Replace the pin's physical line while retaining its surrounding YAML.
	head -n 2 .github/workflows/ci.yml >.github/workflows/ci.yml.head
	cat .github/workflows/ci.yml.head .github/workflows/ci.yml.tmp >.github/workflows/ci.yml
	inspect .github/workflows/ci.yml
	findings_are '[[".github/workflows/ci.yml", "changes a constitutional file"], [".github/workflows/ci.yml", "changes a golden file"]]'
}

@test "AC4 nested unrelated URL before vendir ref preserves findings" {
	cat >vendir.yml <<'YAML'
contents:
  - git:
      url: https://github.com/example/other
      secretRef:
        url: https://github.com/tbhb-dev/devtools
      ref: v0.10.0
YAML
	git add -- vendir.yml
	git commit --quiet -m 'test: nested vendir context'
	BASE=$(git rev-parse HEAD)
	bump vendir.yml
	inspect vendir.yml
	findings_are '[["vendir.yml", "changes a constitutional file"], ["vendir.yml", "changes a golden file"]]'
}

@test "AC4 nested unrelated repository before pre-commit rev preserves findings" {
	cat >.pre-commit-config.yaml <<'YAML'
repos:
  - repo: https://github.com/example/other
    metadata:
      repo: https://github.com/tbhb-dev/devtools
    rev: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  # frozen: v0.10.0
YAML
	git add -- .pre-commit-config.yaml
	git commit --quiet -m 'test: nested pre-commit context'
	BASE=$(git rev-parse HEAD)
	bump .pre-commit-config.yaml
	inspect .pre-commit-config.yaml
	findings_are '[[".pre-commit-config.yaml", "changes a constitutional file"], [".pre-commit-config.yaml", "changes a golden file"]]'
}

@test "AC4 non-comment repository URL suffix preserves findings" {
	printf 'git:\n  url: https://github.com/tbhb-dev/devtools extra\n  ref: v0.10.0\n' >vendir.yml
	git add -- vendir.yml
	git commit --quiet -m 'test: ambiguous URL suffix'
	BASE=$(git rev-parse HEAD)
	bump vendir.yml
	inspect vendir.yml
	findings_are '[["vendir.yml", "changes a constitutional file"], ["vendir.yml", "changes a golden file"]]'
}

@test "AC4 non-comment pre-commit repository suffix preserves findings" {
	printf 'repos:\n  - repo: https://github.com/tbhb-dev/devtools extra\n    rev: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  # frozen: v0.10.0\n' >.pre-commit-config.yaml
	git add -- .pre-commit-config.yaml
	git commit --quiet -m 'test: ambiguous repository suffix'
	BASE=$(git rev-parse HEAD)
	bump .pre-commit-config.yaml
	inspect .pre-commit-config.yaml
	findings_are '[[".pre-commit-config.yaml", "changes a constitutional file"], [".pre-commit-config.yaml", "changes a golden file"]]'
}

@test "AC1 tombi adaptation header replacement has no finding" {
	pin_only tombi.toml
}

@test "AC1 Renovate tag ref replacements have no finding" {
	tag_renovate
	bump .github/renovate.json5
	sed 's/#v0.11.0/#bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/g' .github/renovate.json5 >renovate.tmp
	mv renovate.tmp .github/renovate.json5
	inspect .github/renovate.json5
	findings_are '[]'
}

@test "AC1 mise replacement preserves its comment" {
	sed 's/"v0.10.0"$/"v0.10.0" # tool pin/' mise.toml >mise.tmp
	mv mise.tmp mise.toml
	git add -- mise.toml
	git commit --quiet -m 'test: comment on mise pins'
	BASE=$(git rev-parse HEAD)
	pin_only mise.toml
}

tag_renovate() {
	sed 's/#aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/#v0.10.0/g' .github/renovate.json5 >renovate.tmp
	mv renovate.tmp .github/renovate.json5
	git add -- .github/renovate.json5
	git commit --quiet -m 'test: Renovate tag refs'
	BASE=$(git rev-parse HEAD)
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
	local line
	while IFS= read -r line; do
		if [[ "$line" == '"go:github.com/tbhb-dev/devtools/cmd/guard-markdown"'* ]]; then
			line=${line/\"v0.10.0\"/\"main\"}
		fi
		printf '%s\n' "$line"
	done <mise.toml >mise.toml.tmp
	mv mise.toml.tmp mise.toml
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

@test "AC5 recognized replacement with unchanged suppression preserves all findings" {
	printf '\n"go:github.com/tbhb-dev/devtools/cmd/guard-markdown" = "v0.10.0" # nolint\n' >mise.toml
	git add -- mise.toml
	git commit --quiet -m 'test: existing suppression on a tool pin'
	BASE=$(git rev-parse HEAD)
	bump mise.toml
	inspect mise.toml
	findings_are '[["mise.toml", "changes a constitutional file"], ["mise.toml", "changes a golden file"], ["mise.toml", "adds a suppression or exclusion"]]'
	# The same diff must qualify as a pin replacement independently of the scan.
	run uv run --quiet --script "$POLICY" inspect "$BASE" HEAD <<'JSON'
{"golden_paths":"^mise[.]toml$","loosening_markers":"^$","gate_config_paths":".*","owner_paths":"^mise[.]toml$","disposable_ignored":"^cache/"}
JSON
	[ "$status" -eq 0 ]
	findings_are '[]'
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
	tag_renovate
	# Keep both tag and SHA preset refs in the synchronization fixture.
	local line
	while IFS= read -r line; do
		if [[ "$line" == *'renovate-go-config.json5'* ]]; then
			line=${line/\#v0.10.0/\#aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}
		fi
		printf '%s\n' "$line"
	done <.github/renovate.json5 >renovate.tmp
	mv renovate.tmp .github/renovate.json5
	git add -- .github/renovate.json5
	git commit --quiet -m 'test: both preset ref forms'
	BASE=$(git rev-parse HEAD)
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
'which agentrepo') echo "$BATS_TEST_TMPDIR/bin/agentrepo" ;;
'tasks ls --all') echo 'devtools:check-pins ' ;;
'run devtools:check-pins') exec bash "$BATS_TEST_TMPDIR/check.sh" ;;
esac
STUB
	printf '#!/usr/bin/env bash\nexit 0\n' >"$BATS_TEST_TMPDIR/bin/vendir"
	# update-pins refuses to run without a contract-aware agentrepo. This test
	# measures the pin rewrite, so the tool accepts every contract step; the
	# contract steps themselves run against the real tool in
	# tests/tasks/pins-contract.bats.
	printf '#!/usr/bin/env bash\nexit 0\n' >"$BATS_TEST_TMPDIR/bin/agentrepo"
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
tombi.toml
vendir.yml' ]
	# The task must retain the suffix in both supported comment forms.
	run grep -F '# v0.11.0 extra' .github/workflows/ci.yaml .github/workflows/ci.yml
	[ "$status" -eq 0 ]
	run grep -F '// v0.11.0 extra' .github/renovate.json5
	[ "$status" -eq 0 ]
	inspect apm.yml mise.toml tombi.toml .github vendir.yml .pre-commit-config.yaml
	findings_are '[]'
	[[ "$output" == *'"owner_paths": ["mise.toml"]}' ]]
}
