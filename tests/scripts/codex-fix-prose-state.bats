#!/usr/bin/env bats

setup() {
  ROOT="${BATS_TEST_DIRNAME}/../.."
  CHECK="${ROOT}/packages/agents/codex/.apm/skills/fix-prose/scripts/check-suppressions.sh"
  REPO="${BATS_TEST_TMPDIR}/repo"
  git init --quiet --initial-branch=main "$REPO"
  git -C "$REPO" config user.name Tester
  git -C "$REPO" config user.email tester@example.com
  printf 'first target\n' >"${REPO}/first.md"
  printf 'second target\n' >"${REPO}/second.md"
  printf 'must survive\n' >"${REPO}/other.txt"
  git -C "$REPO" add -- first.md second.md other.txt
  git -C "$REPO" commit --quiet -m 'test: initial state'
}

baseline() {
  local target=$1
  run bash -c 'cd "$1" && bash "$2" --baseline "$3"' _ "$REPO" "$CHECK" "$target"
  [ "$status" -eq 0 ]
  printf '%s\n' "${output#state: }"
}

@test "baselines are unique and bound to their targets" {
  first_state=$(baseline first.md)
  second_state=$(baseline second.md)

  [ "$first_state" != "$second_state" ]
  [ -d "$first_state" ]
  [ -d "$second_state" ]

  run bash -c 'cd "$1" && bash "$2" --verify "$3" second.md' _ "$REPO" "$CHECK" "$first_state"
  [ "$status" -eq 2 ]
  [[ "$output" == *"state belongs to first.md, not second.md"* ]]
}

@test "verify reports deletion of an unrelated clean tracked file" {
  state=$(baseline first.md)
  rm "${REPO}/other.txt"

  run bash -c 'cd "$1" && bash "$2" --verify "$3" first.md' _ "$REPO" "$CHECK" "$state"

  [ "$status" -eq 1 ]
  [[ "$output" == *"other.txt:1 [error] stray-edit"* ]]
}

@test "a second workflow cannot claim an active target" {
  state=$(baseline first.md)

  run bash -c 'cd "$1" && bash "$2" --baseline first.md' _ "$REPO" "$CHECK"

  [ "$status" -eq 2 ]
  [[ "$output" == *"an active workflow already owns first.md"* ]]
  [ -d "$state" ]
}

@test "cleanup removes only the named workflow state" {
  first_state=$(baseline first.md)
  second_state=$(baseline second.md)

  run bash -c 'cd "$1" && bash "$2" --cleanup "$3"' _ "$REPO" "$CHECK" "$first_state"

  [ "$status" -eq 0 ]
  [ ! -e "$first_state" ]
  [ -d "$second_state" ]
}
