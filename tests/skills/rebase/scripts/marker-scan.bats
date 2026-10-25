#!/usr/bin/env bats

setup() {
  bats_require_minimum_version 1.5.0
  source "${BATS_TEST_DIRNAME}/../../../test_helper.bash"
  isolate_git
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
  SCRIPTS="$ROOT/packages/agents/claude/.apm/skills/rebase/scripts"
  git init --quiet --initial-branch=main "$BATS_TEST_TMPDIR/repo"
  cd "$BATS_TEST_TMPDIR/repo"
  git config user.name Tester
  git config user.email tester@example.com
  printf 'one\n' >file
  git add -- file
  git commit --quiet -m 'test: initial state'
  git checkout --quiet -b topic
  printf 'topic\n' >file
  git commit --quiet -am 'test: topic edit'
  git checkout --quiet main
  printf 'main\n' >file
  git commit --quiet -am 'test: main edit'
  git checkout --quiet topic
  unset CLAUDE_CODE_SESSION_ID CODEX_THREAD_ID
  export XDG_CONFIG_HOME="$BATS_TEST_TMPDIR/config"
  export GIT_EDITOR=true REBASE_PREFLIGHT_FETCH=0
}

check_failure() {
  run --separate-stderr ! bash "$SCRIPTS/marker-scan.sh" file
  [ "$status" -ne 0 ]
  [[ "$output" == *"marker-scan.sh: agentrepo markers scan failed"* ]]

  run git rebase main
  [ "$status" -ne 0 ]
  run bash "$SCRIPTS/rebase-status.sh"
  [[ "$output" == *"marker-scan.sh: agentrepo markers scan failed"* ]]
  [[ "$output" == *"conflict markers in the worktree"* ]]

  printf 'resolved\n' >file
  git add -- file
  run bash "$SCRIPTS/continue-rebase.sh"
  [ "$status" -eq 2 ]
  [[ "$output" == *"marker-scan.sh: agentrepo markers scan failed"* ]]
  [[ "$output" == *"Continuing would commit them"* ]]
  git rebase --abort

  printf '<<<<<<< left\n' >file
  git commit --quiet -am 'test: committed marker'
  run bash "$SCRIPTS/verify-rebase.sh" HEAD
  [ "$status" -eq 1 ]
  [[ "$output" == *"marker-scan.sh: agentrepo markers scan failed"* ]]
  [[ "$output" == *"resolution that never finished"* ]]
}

@test "a missing Go scanner reports failure through every caller" {
  export PATH=/usr/bin:/bin
  ! command -v agentrepo
  check_failure
}

@test "a Go scanner that exits without output reports failure through every caller" {
  mkdir "$BATS_TEST_TMPDIR/bin"
  printf '#!/usr/bin/env bash\nexit 1\n' >"$BATS_TEST_TMPDIR/bin/agentrepo"
  chmod +x "$BATS_TEST_TMPDIR/bin/agentrepo"
  export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
  check_failure
}

@test "a scanner startup error stays on stdout even when it prints stderr" {
  mkdir "$BATS_TEST_TMPDIR/bin"
  printf '#!/usr/bin/env bash\nprintf "startup error\\n" >&2\nexit 2\n' >"$BATS_TEST_TMPDIR/bin/agentrepo"
  chmod +x "$BATS_TEST_TMPDIR/bin/agentrepo"
  PATH="$BATS_TEST_TMPDIR/bin:$PATH" run --separate-stderr bash "$SCRIPTS/marker-scan.sh" file
  [ "$status" -eq 2 ]
  [[ "$output" == *"marker-scan.sh: agentrepo markers scan failed"* ]]
  [ "$stderr" = 'startup error' ]
}
