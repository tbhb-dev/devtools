# Exercise the same redesign contract through each real unattended review flow.
# All scratch files live in the Bats-managed directory.
redesign_flow() (
  set -e
  trap - ERR
  local kind=$1 draft=$2 script=$3 state saved round writer
  shift 3
  state=$(git rev-parse --absolute-git-dir)/agent-skills
  saved=$BATS_TEST_TMPDIR/rejected-draft
  trap 'redesign_status=$?; rm -f "$BATS_TEST_TMPDIR/rejected-draft" "$BATS_TEST_TMPDIR/round-history" "$BATS_TEST_TMPDIR/reviewer-calls"; exit "$redesign_status"' EXIT
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo invoked >>"$BATS_TEST_TMPDIR/reviewer-calls"; echo "VERDICT: CHANGES REQUIRED"'
  run agentrepo review redesign "$kind" --reason 'Rewrite the rationale'
  [ "$status" -eq 3 ] || exit 1
  for round in 1 2 3; do
    printf '\nRationale %s.\n' "$round" >>"$draft"
    [ "$round" -ne 1 ] || cp "$draft" "$saved"
    run bash "$script" "$@"
    if [ "$round" -eq 3 ]; then [ "$status" -eq 3 ] || exit 1; else [ "$status" -eq 2 ] || exit 1; fi
    if [ "$round" -eq 1 ]; then
      run agentrepo review redesign "$kind" --reason 'Too early'
      [ "$status" -eq 3 ] || exit 1
    fi
  done
  cp "$state/$kind.rounds" "$BATS_TEST_TMPDIR/round-history"
  run agentrepo review redesign "$kind" --reason 'Rewrite the rationale'
  [ "$status" -eq 0 ] || exit 1
  grep -q '^redesign ' "$state/$kind.rounds" || exit 1
  run bash "$script" "$@"
  [ "$status" -eq 3 ] || exit 1
  [[ "$output" == *"$(cat "$state/$kind.rounds")"* ]] || exit 1
  cp "$saved" "$draft"
  run bash "$script" "$@"
  [ "$status" -eq 3 ] || exit 1
  [ "$(wc -l <"$BATS_TEST_TMPDIR/reviewer-calls" | tr -d ' ')" = 3 ] || exit 1
  for round in 1 2 3; do
    printf '\nNew rationale %s.\n' "$round" >>"$draft"
    run bash "$script" "$@"
    if [ "$round" -eq 3 ]; then
      [ "$status" -eq 3 ] || exit 1
      [[ "$output" == *inspection* ]] || exit 1
    else
      [ "$status" -eq 2 ] || exit 1
    fi
    [[ "$output" == *"$(head -n 1 "$BATS_TEST_TMPDIR/round-history")"* ]] || exit 1
  done
  [ "$(wc -l <"$BATS_TEST_TMPDIR/reviewer-calls" | tr -d ' ')" = 6 ] || exit 1
  run agentrepo review redesign "$kind" --reason 'Another rewrite'
  [ "$status" -eq 3 ] || exit 1
  [[ "$output" == *inspection* ]] || exit 1
  case $kind in
  commit)
    echo another >tracked
    git add -- tracked
    writer=${script%review.sh}commit.sh
    ;;
  pr)
    commit_file another.txt 'fix: advance the branch'
    writer=${script%review.sh}publish.sh
    ;;
  squash)
    change another.txt advanced
    writer=${script%review.sh}merge.sh
    ;;
  esac
  printf '\nChanged subject rationale.\n' >>"$draft"
  run bash "$script" "$@"
  [ "$status" -eq 2 ] || exit 1
  ! grep -q '^redesign ' "$state/$kind.rounds" || exit 1
  run agentrepo review redesign "$kind" --reason 'Not inherited'
  [ "$status" -eq 3 ] || exit 1
  for round in 2 3; do
    printf '\nChanged subject rationale %s.\n' "$round" >>"$draft"
    run bash "$script" "$@"
    if [ "$round" -eq 3 ]; then [ "$status" -eq 3 ] || exit 1; else [ "$status" -eq 2 ] || exit 1; fi
  done
  run agentrepo review redesign "$kind" --reason 'Rewrite the new subject'
  [ "$status" -eq 0 ] || exit 1
  grep -q '^redesign ' "$state/$kind.rounds" || exit 1
  printf '\nFinal rationale.\n' >>"$draft"
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo "VERDICT: PASS"'
  run bash "$script" "$@"
  [ "$status" -eq 0 ] || exit 1
  run bash "$writer" "$@"
  [ "$status" -eq 75 ] || exit 1
  [ -f "$state/$kind.rounds" ] || exit 1
  grant merge
  run bash "$writer" "$@"
  [ "$status" -eq 0 ] || exit 1
  [ ! -e "$state/$kind.rounds" ] || exit 1
)
