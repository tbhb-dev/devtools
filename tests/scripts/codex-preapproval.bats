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
