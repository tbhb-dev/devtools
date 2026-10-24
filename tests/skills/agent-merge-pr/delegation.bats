#!/usr/bin/env bats
setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup
}

recording_checker() {
  export DEVTOOLS_DELEGATION_CHECK="$BATS_TEST_TMPDIR/checker"
  cat >"$DEVTOOLS_DELEGATION_CHECK" <<'CHECK'
#!/usr/bin/env bash
printf '%s\n' "$*" >"$GH_FIXTURES/delegation"
exit 0
CHECK
  chmod +x "$DEVTOOLS_DELEGATION_CHECK"
}

@test "merge delegation receives the PR and pinned head without owner paths" {
  recording_checker
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]

  run bash "$AGENT/merge.sh" 7

  [ "$status" -eq 0 ]
  [ "$(cat "$GH_FIXTURES/delegation")" = "check repo merge --pr 7 --head $HEAD_OID" ]
  grep -q -- "--match-head-commit $HEAD_OID" "$GH_FIXTURES/merged"
}

@test "merge delegation appends owner paths to the PR and pinned head" {
  change owned/input.txt content
  recording_checker
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]

  run bash "$AGENT/merge.sh" 7

  [ "$status" -eq 0 ]
  [ "$(cat "$GH_FIXTURES/delegation")" = "check repo merge --pr 7 --head $HEAD_OID --owner-path" ]
  grep -q -- "--match-head-commit $HEAD_OID" "$GH_FIXTURES/merged"
}

@test "merge delegation remains compatible with a checker reading only its first arguments" {
  export DEVTOOLS_DELEGATION_CHECK="$BATS_TEST_TMPDIR/checker"
  cat >"$DEVTOOLS_DELEGATION_CHECK" <<'CHECK'
#!/usr/bin/env bash
[ "$1" = check ] && [ "$2" = repo ] && [ "$3" = merge ]
CHECK
  chmod +x "$DEVTOOLS_DELEGATION_CHECK"
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]

  run bash "$AGENT/merge.sh" 7

  [ "$status" -eq 0 ]
  grep -q -- "--match-head-commit $HEAD_OID" "$GH_FIXTURES/merged"
}
