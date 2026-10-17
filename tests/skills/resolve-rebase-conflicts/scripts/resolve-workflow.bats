#!/usr/bin/env bats
# The resolve-rebase-conflicts scripts, as generated: which operations
# they accept and what they report when a prerequisite is missing. Each
# case builds a scratch repository whose two branches conflict on one
# line of a sorted word list.

setup() {
  source "${BATS_TEST_DIRNAME}/../../../test_helper.bash"
  isolate_git
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
  SCRIPTS="${ROOT}/packages/agents/claude/.apm/skills/resolve-rebase-conflicts/scripts"
  REPO="${BATS_TEST_TMPDIR}/repo"
  export XDG_CONFIG_HOME="${BATS_TEST_TMPDIR}/config"
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 GIT_EDITOR=true
  git init --quiet --initial-branch=main "$REPO"
  git -C "$REPO" config user.name Tester
  git -C "$REPO" config user.email tester@example.com
  cd "$REPO"
  printf 'alpha\nomega\n' >words.txt
  git add -- words.txt
  git commit --quiet -m 'test: initial state'
  git checkout --quiet -b topic
  printf 'alpha\nbeta\nomega\n' >words.txt
  git commit --quiet -am 'test: add beta'
  git checkout --quiet main
  printf 'alpha\ngamma\nomega\n' >words.txt
  git commit --quiet -am 'test: add gamma'
  git checkout --quiet topic
}

@test "classify reads a conflict stopped under the apply backend" {
  run git rebase --apply main
  [ "$status" -ne 0 ]

  run bash "$SCRIPTS/classify-conflicts.sh"

  [ "$status" -eq 0 ]
  [[ "$output" == *"sorted-union"* ]]
}

@test "classify reads a stopped cherry-pick" {
  git checkout --quiet main
  run git cherry-pick topic
  [ "$status" -ne 0 ]

  run bash "$SCRIPTS/classify-conflicts.sh"

  [ "$status" -eq 0 ]
  [[ "$output" == *"sorted-union"* ]]
}

@test "resolve-union settles a stopped cherry-pick" {
  git checkout --quiet main
  run git cherry-pick topic
  [ "$status" -ne 0 ]

  run bash "$SCRIPTS/resolve-union.sh" words.txt

  [ "$status" -eq 0 ]
  [ "$(cat words.txt)" = "$(printf 'alpha\nbeta\ngamma\nomega')" ]
}

@test "classify says plainly that no rebase or cherry-pick is running" {
  run bash "$SCRIPTS/classify-conflicts.sh"

  [ "$status" -eq 1 ]
  [[ "$output" == *"no rebase or cherry-pick in progress"* ]]
}

@test "classify names uv when it is missing" {
  run git rebase main
  fake="${BATS_TEST_TMPDIR}/bin"
  mkdir -p "$fake"
  for tool in git bash sed awk tr cat dirname mktemp rm sort uniq comm grep; do
    ln -s "$(command -v "$tool")" "$fake/$tool"
  done

  PATH="$fake" run /bin/bash "$SCRIPTS/classify-conflicts.sh"

  [ "$status" -eq 1 ]
  [[ "$output" == *"uv is not installed"* ]]
}

@test "resolve-union names python3 when it is missing" {
  run git rebase main
  fake="${BATS_TEST_TMPDIR}/bin"
  mkdir -p "$fake"
  for tool in git bash sed awk tr cat dirname mktemp rm sort uniq comm grep uv; do
    ln -s "$(command -v "$tool")" "$fake/$tool"
  done

  PATH="$fake" run /bin/bash "$SCRIPTS/resolve-union.sh" words.txt

  [ "$status" -eq 1 ]
  [[ "$output" == *"python3 is not installed"* ]]
}

@test "scripts find conflict_shape.py when run by a relative path from a subdirectory" {
  mkdir sub
  run git rebase main
  [ "$status" -ne 0 ]
  cd sub
  abs=$(cd "$SCRIPTS" && pwd -P)
  rel=$(python3 -c 'import os, sys; print(os.path.relpath(sys.argv[1], sys.argv[2]))' "$abs" "$(pwd -P)")

  run bash "$rel/classify-conflicts.sh"

  [ "$status" -eq 0 ]
  [[ "$output" == *"sorted-union"* ]]
  [[ "$output" == *"$abs/resolve-union.sh"* ]]
}
