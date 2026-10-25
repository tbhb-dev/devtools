#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  pr_setup
}

@test "AC1 AC5 AC6 attended close execs prctl with shared authority and preserves the guard" {
  rm PR_AGENTDESC.md
  git checkout -q main
  export DEVTOOLS_GH_LOGIN=agent
  export PRCTL_LOG="$BATS_TEST_TMPDIR/prctl.log"
  cat >"$BATS_TEST_TMPDIR/bin/prctl" <<'STUB'
#!/usr/bin/env bash
printf '%s %s\n' "${DEVTOOLS_PR_CLOSE_GRANTED:-missing}" "$*" >>"$PRCTL_LOG"
exit 23
STUB
  chmod +x "$BATS_TEST_TMPDIR/bin/prctl"
  run bash "$SCRIPTS/create-pr.sh" close 1 --duplicate-of 2 --confirmed
  [ "$status" -eq 23 ]
  grep -qx '1 close 1 --duplicate-of 2' "$PRCTL_LOG"
  [ ! -s "$GATE_LOG" ]
  refute_grep 'pr create\|pr edit\|pr ready' "$GH_LOG"

  export CODEX_THREAD_ID=close-session
  mkdir -p .git/preapprovals
  printf 'pr now unconfirmed\n' >.git/preapprovals/close-session
  run bash "$SCRIPTS/create-pr.sh" close 1 --duplicate-of 2
  [ "$status" -eq 23 ]
  [[ "$output" == *'confirmation: pre-approval'* ]]
  rm .git/preapprovals/close-session
  export DEVTOOLS_DELEGATION_CHECK="$BATS_TEST_TMPDIR/checker"
  printf '#!/usr/bin/env bash\n[ "$3" = pr ]\n' >"$DEVTOOLS_DELEGATION_CHECK"
  chmod +x "$DEVTOOLS_DELEGATION_CHECK"
  run bash "$SCRIPTS/create-pr.sh" close 1 --duplicate-of 2
  [ "$status" -eq 23 ]
  [[ "$output" == *'confirmation: delegation_check'* ]]

  export DEVTOOLS_GH_LOGIN=other
  run bash "$SCRIPTS/create-pr.sh" close 1 --duplicate-of 2 --confirmed
  [ "$status" -eq 1 ]
  [ "$(wc -l <"$PRCTL_LOG")" -eq 3 ]
  run bash "$SCRIPTS/guard-gh.sh" <<<'{"tool_input":{"command":"gh pr close 1"}}'
  [ "$status" -eq 2 ]
  [[ "$output" == *'prctl close'* ]]
}
