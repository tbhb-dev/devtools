#!/usr/bin/env bats
# The fix-prose family's scripts, as generated: the suppression check with
# its per-run state, the writer's context script, and the Claude guard's
# one-edit release. Each case runs in a scratch repository with no global
# git configuration. State and notes must live inside the worktree, so the
# cases also withdraw write access to the git directory where they can.

setup() {
  source "${BATS_TEST_DIRNAME}/../../../test_helper.bash"
  isolate_git
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
  SKILL="${ROOT}/packages/agents/claude/.apm/skills/fix-prose/scripts"
  WRITER="${ROOT}/packages/agents/claude/.apm/skills/write-prose-fix/scripts"
  REPO="${BATS_TEST_TMPDIR}/repo"
  unset CLAUDE_CODE_SESSION_ID CODEX_THREAD_ID CLAUDECODE DEVTOOLS_HARNESS
  export XDG_CONFIG_HOME="${BATS_TEST_TMPDIR}/config"
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
  git init --quiet --initial-branch=main "$REPO"
  git -C "$REPO" config user.name Tester
  git -C "$REPO" config user.email tester@example.com
  cd "$REPO"
  mkdir -p .vale/config/vocabularies/project
  printf 'alpha\nbravo\n' >.cspell-words.txt
  printf 'one\ntwo\n' >.vale/config/vocabularies/project/accept.txt
  printf 'StylesPath = .vale\n' >.vale.ini
  printf 'first draft\n' >DRAFT.md
  git add -- .cspell-words.txt .vale .vale.ini DRAFT.md
  git commit --quiet -m 'test: initial state'
}

baseline() {
  run bash "$SKILL/check-suppressions.sh" --baseline "${1:-DRAFT.md}"
  [ "$status" -eq 0 ]
  STATE=${output#state: }
}

@test "the baseline writes inside the worktree when the git directory is read-only (#notes)" {
  git_dir=$(git rev-parse --absolute-git-dir)
  chmod a-w "$git_dir"

  run bash "$SKILL/check-suppressions.sh" --baseline DRAFT.md

  chmod u+w "$git_dir"
  [ "$status" -eq 0 ]
  [[ "$output" == "state: $(pwd -P)/.fix-prose/run."* ]]
  [ -z "$(git status --porcelain)" ]
}

@test "the state directory keeps a handed-over decision without disturbing the verification" {
  baseline
  printf 'keep the name\n' >"$STATE/notes"
  printf 'second draft\n' >DRAFT.md

  run bash "$SKILL/check-suppressions.sh" --verify "$STATE" DRAFT.md

  [ "$status" -eq 0 ]
  [ -z "$output" ]
}

@test "the writer's context prints notes from the state, then from the old git directory path" {
  baseline
  printf 'decision from the state\n' >"$STATE/notes"
  run bash "$WRITER/context.sh" DRAFT.md "$STATE" true
  [[ "$output" == *"what the caller asked you to address"* ]]
  [[ "$output" == *"decision from the state"* ]]

  rm "$STATE/notes"
  printf 'decision from the old path\n' >"$(git rev-parse --absolute-git-dir)/fix-prose.notes"
  run bash "$WRITER/context.sh" DRAFT.md "$STATE" true
  [[ "$output" == *"decision from the old path"* ]]
}

@test "a state under the git directory from an earlier version still loads" {
  git_dir=$(git rev-parse --absolute-git-dir)
  legacy="$git_dir/fix-prose/run.legacy"
  mkdir -p "$legacy"
  printf 'DRAFT.md\n' >"$legacy/target"
  : >"$legacy/digest"

  run bash "$SKILL/check-suppressions.sh" --cleanup "$legacy"

  [ "$status" -eq 0 ]
  [ ! -d "$legacy" ]
}

@test "two targets get distinct states and a second run on one target is refused" {
  printf 'other\n' >OTHER.md
  baseline DRAFT.md
  first=$STATE
  baseline OTHER.md
  [ "$first" != "$STATE" ]

  run bash "$SKILL/check-suppressions.sh" --baseline DRAFT.md

  [ "$status" -eq 2 ]
  [[ "$output" == *"--cleanup"* ]]
}

@test "verify flags a dictionary addition nobody sanctioned" {
  baseline
  printf 'charlie\n' >>.cspell-words.txt

  run bash "$SKILL/check-suppressions.sh" --verify "$STATE" DRAFT.md

  [ "$status" -eq 1 ]
  [[ "$output" == *"linter-config-changed"* ]]
}

@test "verify passes on exactly the added lines the operator sanctioned" {
  baseline
  printf 'charlie\n' >>.cspell-words.txt
  git add -- .cspell-words.txt
  printf 'delta\n' >>.vale/config/vocabularies/project/accept.txt

  run bash "$SKILL/check-suppressions.sh" --allow "$STATE" .cspell-words.txt charlie
  [ "$status" -eq 0 ]
  run bash "$SKILL/check-suppressions.sh" --allow "$STATE" .vale/config/vocabularies/project/accept.txt delta
  [ "$status" -eq 0 ]
  run bash "$SKILL/check-suppressions.sh" --verify "$STATE" DRAFT.md

  [ "$status" -eq 0 ]
  [ -z "$output" ]
}

@test "verify still fails on an extra word beside a sanctioned one" {
  baseline
  printf 'charlie\nsneaky\n' >>.cspell-words.txt
  bash "$SKILL/check-suppressions.sh" --allow "$STATE" .cspell-words.txt charlie

  run bash "$SKILL/check-suppressions.sh" --verify "$STATE" DRAFT.md

  [ "$status" -eq 1 ]
  [[ "$output" == *"did not allow: sneaky"* ]]
  [[ "$output" != *"did not allow: charlie"* ]]
}

@test "verify still fails on a removed word beside a sanctioned addition" {
  baseline
  printf 'bravo\ncharlie\n' >.cspell-words.txt
  bash "$SKILL/check-suppressions.sh" --allow "$STATE" .cspell-words.txt charlie

  run bash "$SKILL/check-suppressions.sh" --verify "$STATE" DRAFT.md

  [ "$status" -eq 1 ]
  [[ "$output" == *"removed the line: alpha"* ]]
}

@test "a rule edit fails whether or not a word was sanctioned, and the rule files cannot be sanctioned" {
  baseline
  printf 'charlie\n' >>.cspell-words.txt
  printf 'MinAlertLevel = suggestion\n' >>.vale.ini
  bash "$SKILL/check-suppressions.sh" --allow "$STATE" .cspell-words.txt charlie

  run bash "$SKILL/check-suppressions.sh" --verify "$STATE" DRAFT.md
  [ "$status" -eq 1 ]
  [[ "$output" == *".vale.ini:1 [error] linter-config-changed"* ]]

  run bash "$SKILL/check-suppressions.sh" --allow "$STATE" .vale.ini 'MinAlertLevel = suggestion'
  [ "$status" -eq 2 ]
  [[ "$output" == *"cannot be sanctioned"* ]]
}

@test "the diff for the reviewer lists what the operator sanctioned" {
  baseline
  printf 'charlie\n' >>.cspell-words.txt
  bash "$SKILL/check-suppressions.sh" --allow "$STATE" .cspell-words.txt charlie
  printf 'second draft\n' >DRAFT.md

  run bash "$SKILL/check-suppressions.sh" --diff "$STATE" DRAFT.md

  [[ "$output" == *"+second draft"* ]]
  [[ "$output" == *"sanctioned by the operator"* ]]
  [[ "$output" == *".cspell-words.txt: charlie"* ]]
}

@test "an edit to a file other than the target is a stray edit" {
  baseline
  printf 'x\n' >NEW.md

  run bash "$SKILL/check-suppressions.sh" --verify "$STATE" DRAFT.md

  [ "$status" -eq 1 ]
  [[ "$output" == *"stray-edit"* ]]
}

@test "the one-edit release works with a read-only git directory and the guard consumes it" {
  command -v jq >/dev/null || skip "jq is not installed"
  git_dir=$(git rev-parse --absolute-git-dir)
  printf '%s\n' "$(pwd -P)/DRAFT.md" >"$git_dir/fix-prose.lock"
  payload=$(jq -n --arg cwd "$(pwd -P)" --arg f "$(pwd -P)/DRAFT.md" '{cwd:$cwd,tool_input:{file_path:$f}}')

  run bash "$SKILL/guard-target.sh" <<<"$payload"
  [ "$status" -eq 2 ]

  chmod a-w "$git_dir"
  run bash "$SKILL/release-once.sh"
  chmod u+w "$git_dir"
  [ "$status" -eq 0 ]
  [ -e .fix-prose/release ]

  run bash "$SKILL/guard-target.sh" <<<"$payload"
  [ "$status" -eq 0 ]
  run bash "$SKILL/guard-target.sh" <<<"$payload"
  [ "$status" -eq 2 ]
}
