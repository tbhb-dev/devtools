#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_pr_setup
}

@test "agent PR rejects the operator flag before publication" {
  run bash "$SCRIPTS/publish.sh" --confirmed
  [ "$status" -eq 1 ]
  [[ "$output" == *'refuse --confirmed'* ]]
}

@test "agent PR requires a machine account instead of skipping identity" {
  unset DEVTOOLS_GH_LOGIN
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'gh_login must be configured'* ]]
}

@test "agent PR refuses dirty or interrupted branches before review" {
  echo uncommitted >untracked
  run bash "$SCRIPTS/review.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'Commit the work first'* ]]
  rm untracked
  touch .git/REVERT_HEAD
  run bash "$SCRIPTS/review.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'REVERT_HEAD'* ]]
}

@test "agent PR holds without authority and never asks" {
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 75 ]
  [[ "$output" == *'without prompting'* ]]
  [[ "$output" != *'Ask the operator'* ]]
  [ -z "$(remote_topic)" ]
}

@test "agent PR refuses a mismatched machine identity before write" {
  export DEVTOOLS_GH_LOGIN=another-agent
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'not another-agent'* ]]
  [ -z "$(remote_topic)" ]
}

@test "agent PR delegates protected paths across the whole branch" {
  echo 'owner_paths = ^docs/' >.agents/agent-skills.conf
  git mv docs/gone.md moved.md
  git commit -qm 'fix: move the document'
  commit_file other 'fix: update another file'
  export DEVTOOLS_DELEGATION_CHECK="$BATS_TEST_TMPDIR/checker"
  cat >"$DEVTOOLS_DELEGATION_CHECK" <<'CHECKER'
#!/usr/bin/env bash
printf '%s\n' "$*" >"$CHECK_ARGS"
exit "${CHECK_RESULT:-1}"
CHECKER
  chmod +x "$DEVTOOLS_DELEGATION_CHECK"
  export CHECK_ARGS="$BATS_TEST_TMPDIR/check.args"
  bash "$SCRIPTS/review.sh"
  CHECK_RESULT=1 run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 1 ]
  [[ "$(cat "$CHECK_ARGS")" == *' pr --owner-path' ]]
  [ -z "$(remote_topic)" ]
  CHECK_RESULT=9 run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 75 ]
  CHECK_RESULT=0 run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 0 ]
  [ "$(remote_topic)" = "$(git rev-parse HEAD)" ]
}

@test "agent PR refuses a branch switch by the delegation checker" {
  git branch alternate
  export DEVTOOLS_DELEGATION_CHECK="$BATS_TEST_TMPDIR/checker"
  printf '#!/usr/bin/env bash\ngit switch -q alternate\n' >"$DEVTOOLS_DELEGATION_CHECK"
  chmod +x "$DEVTOOLS_DELEGATION_CHECK"
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'branch changed'* || "$output" == *'branch changed after'* ]]
  [ -z "$(remote_topic)" ]
}

@test "agent PR findings stay blocked at the configured bound" {
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo "VERDICT: CHANGES REQUIRED"'
  round=0
  for expected in 2 2 3 3; do
    round=$((round + 1))
    printf '\nReview round %s.\n' "$round" >>PR_AGENTDESC.md
    run bash "$SCRIPTS/review.sh"
    [ "$status" -eq "$expected" ]
  done
}

@test "agent PR gives a new head fresh finding rounds" {
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo "VERDICT: CHANGES REQUIRED"'
  for round in 1 2 3; do
    printf '\nReview round %s.\n' "$round" >>PR_AGENTDESC.md
    run bash "$SCRIPTS/review.sh"
  done
  [ "$status" -eq 3 ]

  commit_file another.txt 'fix: advance the branch'
  run bash "$SCRIPTS/review.sh"
  [ "$status" -eq 2 ]
}

@test "agent PR refuses detached and empty branches" {
  git checkout -q --detach
  run bash "$SCRIPTS/review.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *detached* ]]
  git checkout -qb empty origin/main
  run bash "$SCRIPTS/preflight.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'No commits to publish'* ]]
}

@test "agent PR preflight preserves an unpublished draft and exhausted bound" {
  printf 'draft marker\n' >>PR_AGENTDESC.md
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo "VERDICT: CHANGES REQUIRED"'
  round=0
  for expected in 2 2 3; do
    round=$((round + 1))
    printf '\nReview round %s.\n' "$round" >>PR_AGENTDESC.md
    run bash "$SCRIPTS/review.sh"
    [ "$status" -eq "$expected" ]
  done
  bash "$SCRIPTS/preflight.sh"
  grep -q 'draft marker' PR_AGENTDESC.md
  grep -q '^rounds 3$' .git/agent-skills/pr.rounds
  run bash "$SCRIPTS/review.sh"
  [ "$status" -eq 3 ]
}

@test "agent PR owner matching preserves newline paths and regex alternatives" {
  export DEVTOOLS_DELEGATION_CHECK="$BATS_TEST_TMPDIR/checker"
  export CHECK_ARGS="$BATS_TEST_TMPDIR/check.args"
  printf '#!/usr/bin/env bash\nprintf '%%s\n' "$*" >"$CHECK_ARGS"\nexit 1\n' >"$DEVTOOLS_DELEGATION_CHECK"
  chmod +x "$DEVTOOLS_DELEGATION_CHECK"
  echo 'owner_paths = ^(absent/.*|docs/.*name)$' >.agents/agent-skills.conf
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 1 ]
  refute_grep -- '--owner-path' "$CHECK_ARGS"
  commit_file $'docs/private\nname' 'fix: add a protected document'
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 1 ]
  grep -q -- '--owner-path' "$CHECK_ARGS"
}

@test "agent PR invalid owner expression blocks even with preapproval" {
  grant
  echo 'owner_paths = [' >.agents/agent-skills.conf
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'Invalid owner_paths'* ]]
  [ -z "$(remote_topic)" ]
}
