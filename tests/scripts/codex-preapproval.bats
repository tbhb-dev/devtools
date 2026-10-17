#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  ROOT="${BATS_TEST_DIRNAME}/../.."
  REPO="${BATS_TEST_TMPDIR}/repo"
  git init --quiet --initial-branch=main "$REPO"
  GIT_DIR_PATH=$(git -C "$REPO" rev-parse --absolute-git-dir)
}

run_task() {
  local task=$1
  shift
  run env -u CLAUDE_CODE_SESSION_ID CODEX_THREAD_ID=codex-test-thread \
    GIT_DIR="$GIT_DIR_PATH" GIT_WORK_TREE="$REPO" \
    mise --cd "$ROOT" run "$task" "$@"
}

@test "preapprove and revoke fall back to CODEX_THREAD_ID" {
  run_task preapprove --no-biometrics commit
  [ "$status" -eq 0 ]
  [ -f "${GIT_DIR_PATH}/preapprovals/codex-test-thread" ]
  grep -q '^# Pre-approval record for Codex thread codex-test-thread\.$' "${GIT_DIR_PATH}/preapprovals/codex-test-thread"
  grep -q '^commit ' "${GIT_DIR_PATH}/preapprovals/codex-test-thread"

  run_task revoke-preapproval
  [ "$status" -eq 0 ]
  [ ! -e "${GIT_DIR_PATH}/preapprovals/codex-test-thread" ]
}

@test "CLAUDE_CODE_SESSION_ID remains preferred when both are set" {
  run env CLAUDE_CODE_SESSION_ID=claude-test-session CODEX_THREAD_ID=codex-test-thread \
    GIT_DIR="$GIT_DIR_PATH" GIT_WORK_TREE="$REPO" \
    mise --cd "$ROOT" run preapprove --no-biometrics commit

  [ "$status" -eq 0 ]
  [ -f "${GIT_DIR_PATH}/preapprovals/claude-test-session" ]
  grep -q '^# Pre-approval record for Claude Code session claude-test-session\.$' "${GIT_DIR_PATH}/preapprovals/claude-test-session"
  [ ! -e "${GIT_DIR_PATH}/preapprovals/codex-test-thread" ]
}

@test "declared consumer scope can be granted checked and revoked" {
  export DEVTOOLS_PREAPPROVAL_SCOPES='apply deploy-hosts'
  run_task preapprove --no-biometrics apply
  [ "$status" -eq 0 ]
  run_task check-preapproval apply
  [ "$status" -eq 0 ]
  run_task check-preapproval deploy-hosts
  [ "$status" -eq 1 ]
  run_task revoke-preapproval apply
  [ "$status" -eq 0 ]
  run_task check-preapproval apply
  [ "$status" -eq 1 ]
}

@test "all grants and revokes only shared scopes" {
  export DEVTOOLS_PREAPPROVAL_SCOPES=apply
  run_task preapprove --no-biometrics all
  [ "$status" -eq 0 ]
  for scope in commit pr merge; do
    run_task check-preapproval "$scope"
    [ "$status" -eq 0 ]
  done
  run_task check-preapproval apply
  [ "$status" -eq 1 ]
  run_task preapprove --no-biometrics apply
  [ "$status" -eq 0 ]
  run_task revoke-preapproval all
  [ "$status" -eq 0 ]
  run_task check-preapproval apply
  [ "$status" -eq 0 ]
  run_task check-preapproval commit
  [ "$status" -eq 1 ]
}

@test "invalid and undeclared names fail without changing grants" {
  export DEVTOOLS_PREAPPROVAL_SCOPES=apply
  run_task preapprove --no-biometrics apply
  [ "$status" -eq 0 ]
  record="${GIT_DIR_PATH}/preapprovals/codex-test-thread"
  cp "$record" "$BATS_TEST_TMPDIR/before"
  for scope in Apply '../apply' 'apply.*' undeclared ''; do
    for task in preapprove revoke-preapproval check-preapproval; do
      if [ "$task" = preapprove ]; then
        run_task "$task" --no-biometrics "$scope"
      else
        run_task "$task" "$scope"
      fi
      [ "$status" -ne 0 ]
      cmp "$record" "$BATS_TEST_TMPDIR/before"
    done
  done
  for config in 'apply Bad' 'all' '../apply' 'd*'; do
    export DEVTOOLS_PREAPPROVAL_SCOPES=$config
    run_task preapprove --no-biometrics commit
    [ "$status" -ne 0 ]
    cmp "$record" "$BATS_TEST_TMPDIR/before"
  done
}

@test "consumer grants remain isolated to their session" {
  export DEVTOOLS_PREAPPROVAL_SCOPES=apply
  run_task preapprove --no-biometrics apply
  [ "$status" -eq 0 ]
  run env -u CLAUDE_CODE_SESSION_ID CODEX_THREAD_ID=another-thread \
    DEVTOOLS_PREAPPROVAL_SCOPES=apply GIT_DIR="$GIT_DIR_PATH" GIT_WORK_TREE="$REPO" \
    mise --cd "$ROOT" run check-preapproval apply
  [ "$status" -eq 1 ]
  run env -u CLAUDE_CODE_SESSION_ID CODEX_THREAD_ID=another-thread \
    GIT_DIR="$GIT_DIR_PATH" GIT_WORK_TREE="$REPO" \
    mise --cd "$ROOT" run revoke-preapproval
  [ "$status" -eq 0 ]
  run_task check-preapproval apply
  [ "$status" -eq 0 ]
}

@test "configured names accept whitespace without treating patterns as names" {
  export DEVTOOLS_PREAPPROVAL_SCOPES=$'apply\tdeploy-hosts\n'
  run_task preapprove --no-biometrics apply deploy-hosts
  [ "$status" -eq 0 ]
  run_task check-preapproval deploy-hosts
  [ "$status" -eq 0 ]
  run_task revoke-preapproval apply undeclared
  [ "$status" -ne 0 ]
  run_task check-preapproval apply
  [ "$status" -eq 0 ]
}

@test "empty consumer scopes work under system bash with unset-variable checks" {
  run env DEVTOOLS_PREAPPROVAL_SCOPES= /bin/bash -uc '
    source "$1/.devtools/scripts/preapproval.sh"
    preapproval_scopes commit pr merge all
  ' bash "$ROOT"
  [ "$status" -eq 0 ]
  [ "$output" = $'commit\npr\nmerge\ncommit\npr\nmerge' ]

  run env DEVTOOLS_PREAPPROVAL_SCOPES= /bin/bash -uc '
    source "$1/.devtools/scripts/preapproval.sh"
    preapproval_scopes undeclared
  ' bash "$ROOT"
  [ "$status" -eq 1 ]
  [ "$output" = "unknown scope 'undeclared'" ]
}
