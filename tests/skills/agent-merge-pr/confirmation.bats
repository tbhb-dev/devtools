#!/usr/bin/env bats
setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup
}

confirmation_source() {
  case $1 in
  operator) MERGE_ARGS=(--confirmed 7) ;;
  preapproval) grant merge; MERGE_ARGS=(7) ;;
  delegation)
    export DEVTOOLS_DELEGATION_CHECK="$BATS_TEST_TMPDIR/checker"
    printf '#!/usr/bin/env bash\nexit 0\n' >"$DEVTOOLS_DELEGATION_CHECK"
    chmod +x "$DEVTOOLS_DELEGATION_CHECK"
    MERGE_ARGS=(7)
    ;;
  esac
}

merge_gate_case() {
  local source=$1 gate=$2 expected=$3
  confirmation_source "$source"
  case $gate in
  golden) change golden/result.txt baseline ;;
  loosening) change lint.yaml 'ignore: [rule]' ;;
  esac
  # Attended review cannot know how execution will be confirmed.
  bash "$SCRIPTS/review.sh" 7
  case $gate in
  reviewer) pr '.reviews[0].commit.oid="old"' ;;
  label) pr '.labels=[{name:"owner-review"}]' ;;
  requirements) echo '[]' >"$GH_FIXTURES/api/repos_{owner}_{repo}_rules_branches_main.json" ;;
  checks) pr '.statusCheckRollup[0].conclusion="SKIPPED"' ;;
  empty) pr '.statusCheckRollup=[]' ;;
  esac
  run bash "$SCRIPTS/squash-merge.sh" "${MERGE_ARGS[@]}"
  if [ "$source" = operator ] && [ "$gate" != empty ]; then
    [ "$status" -eq 0 ]
    [ -f "$GH_FIXTURES/merged" ]
  else
    [ "$status" -ne 0 ]
    [[ "$output" == *"$expected"* ]] || { printf '%s\n' "$output"; return 1; }
    [ ! -f "$GH_FIXTURES/merged" ]
    [[ "$output" != *"open, ready, mergeable, and green"* || "$gate" != empty ]]
  fi
}

@test "operator confirmation enforces reviewer policy as required" {
  merge_gate_case operator reviewer "WAITING_REVIEW"
}

@test "operator confirmation enforces label policy as required" {
  merge_gate_case operator label "needs-owner"
}

@test "operator confirmation enforces golden policy as required" {
  merge_gate_case operator golden "needs-owner"
}

@test "operator confirmation enforces loosening policy as required" {
  merge_gate_case operator loosening "needs-owner"
}

@test "operator confirmation enforces requirements policy as required" {
  merge_gate_case operator requirements "merge policy unreadable"
}

@test "operator confirmation enforces checks policy as required" {
  merge_gate_case operator checks '"outcome":1'
}

@test "operator confirmation enforces empty policy as required" {
  merge_gate_case operator empty "no checks reported"
}

@test "preapproval confirmation enforces reviewer policy as required" {
  merge_gate_case preapproval reviewer "WAITING_REVIEW"
}

@test "preapproval confirmation enforces label policy as required" {
  merge_gate_case preapproval label "needs-owner"
}

@test "preapproval confirmation enforces golden policy as required" {
  merge_gate_case preapproval golden "needs-owner"
}

@test "preapproval confirmation enforces loosening policy as required" {
  merge_gate_case preapproval loosening "needs-owner"
}

@test "preapproval confirmation enforces requirements policy as required" {
  merge_gate_case preapproval requirements "merge policy unreadable"
}

@test "preapproval confirmation enforces checks policy as required" {
  merge_gate_case preapproval checks '"outcome":1'
}

@test "preapproval confirmation enforces empty policy as required" {
  merge_gate_case preapproval empty "no checks reported"
}

@test "preapproval confirmation merges when all gates pass" {
  confirmation_source preapproval
  bash "$SCRIPTS/review.sh" 7
  run bash "$SCRIPTS/squash-merge.sh" "${MERGE_ARGS[@]}"
  [ "$status" -eq 0 ]
  [ -f "$GH_FIXTURES/merged" ]
}

@test "delegation confirmation enforces reviewer policy as required" {
  merge_gate_case delegation reviewer "WAITING_REVIEW"
}

@test "delegation confirmation enforces label policy as required" {
  merge_gate_case delegation label "needs-owner"
}

@test "delegation confirmation enforces golden policy as required" {
  merge_gate_case delegation golden "needs-owner"
}

@test "delegation confirmation enforces loosening policy as required" {
  merge_gate_case delegation loosening "needs-owner"
}

@test "delegation confirmation enforces requirements policy as required" {
  merge_gate_case delegation requirements "merge policy unreadable"
}

@test "delegation confirmation enforces checks policy as required" {
  merge_gate_case delegation checks '"outcome":1'
}

@test "delegation confirmation enforces empty policy as required" {
  merge_gate_case delegation empty "no checks reported"
}

@test "delegation confirmation merges when all gates pass" {
  confirmation_source delegation
  bash "$SCRIPTS/review.sh" 7
  run bash "$SCRIPTS/squash-merge.sh" "${MERGE_ARGS[@]}"
  [ "$status" -eq 0 ]
  [ -f "$GH_FIXTURES/merged" ]
}

@test "legacy unattended environment alias works and the new name takes precedence" {
  run env DT_UNATTENDED=1 bash "$SCRIPTS/squash-merge.sh" --confirmed 7
  [ "$status" -ne 0 ]
  [[ "$output" == *refuse*--confirmed* ]]
  run env DEVTOOLS_UNATTENDED=1 bash "$SCRIPTS/squash-merge.sh" --confirmed 7
  [ "$status" -ne 0 ]
  [[ "$output" == *refuse*--confirmed* ]]
  bash "$SCRIPTS/review.sh" 7
  run env DEVTOOLS_UNATTENDED=0 DT_UNATTENDED=1 bash "$SCRIPTS/squash-merge.sh" --confirmed 7
  [ "$status" -eq 0 ]
}

@test "preflight reports no checks as inconclusive" {
  pr '.statusCheckRollup=[]'
  run bash "$SCRIPTS/preflight.sh" 7
  [ "$status" -eq 0 ]
  [[ "$output" == *"no checks reported"* ]]
  [[ "$output" != *"all green"* ]]
}

@test "squash-merge holds with no confirmation from anyone" {
  bash "$SCRIPTS/review.sh" 7

  run bash "$SCRIPTS/squash-merge.sh" 7

  [ "$status" -eq 75 ]
  [[ "$output" == *"HELD"* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}


@test "squash-merge holds under a bare pre-approval, which never covers merge" {
  bash "$SCRIPTS/review.sh" 7
  grant commit pr

  run bash "$SCRIPTS/squash-merge.sh" 7

  [ "$status" -eq 75 ]
  [ ! -e "$GH_FIXTURES/merged" ]
}


@test "squash-merge merges under a merge pre-approval, pinned to the reviewed head" {
  bash "$SCRIPTS/review.sh" 7
  grant merge
  printf 'draft\n' >PR_AGENTDESC.md

  run bash "$SCRIPTS/squash-merge.sh" 7

  [ "$status" -eq 0 ]
  [[ "$output" == *"confirmation: pre-approval"* ]]
  [[ "$output" == *"signed in as agent"* ]]
  grep -q -- "--match-head-commit $HEAD_OID" "$GH_FIXTURES/merged"
  grep -q -- "--subject feat: add the thing (#7)" "$GH_FIXTURES/merged"
  grep -q -- "--body-file" "$GH_FIXTURES/merged"
  [ ! -e SQUASH_AGENTMSG ]
  [ ! -e PR_AGENTDESC.md ]
  [ ! -e "$(stamp_file)" ]
}


@test "delegated merge checks its script-owned stamp and checker refusal then grant (#137)" {
  # Opt-in CLI smoke test; ordinary runs keep the deterministic reviewer.
  if [ "${RUN_HEADLESS_REVIEWS:-0}" = 1 ]; then
    unset DEVTOOLS_REVIEW_CMD
    export DEVTOOLS_HARNESS=claude
  fi
  # No skill hooks, operator flag, or session pre-approval participates.
  [ -z "${CLAUDE_CODE_SESSION_ID:-}${CODEX_THREAD_ID:-}" ]
  export DEVTOOLS_DELEGATION_CHECK="${BATS_TEST_TMPDIR}/checker"
  export CHECK_ARGS="${BATS_TEST_TMPDIR}/checker.args"
  cat >"$DEVTOOLS_DELEGATION_CHECK" <<'CHECKER'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$CHECK_ARGS"
exit "${CHECK_STATUS:-1}"
CHECKER
  chmod +x "$DEVTOOLS_DELEGATION_CHECK"
  bash "$SCRIPTS/review.sh" 7
  [ -s "$(stamp_file)" ]

  CHECK_STATUS=1 run bash "$SCRIPTS/squash-merge.sh" 7
  [ "$status" -eq 1 ]
  [[ "$output" == *REFUSED* ]]
  [ -s "$(stamp_file)" ]

  CHECK_STATUS=0 run bash "$SCRIPTS/squash-merge.sh" 7
  [ "$status" -eq 0 ]
  [[ "$output" == *"confirmation: delegation_check granted merge in repo"* ]]
  [ "$(cat "$CHECK_ARGS")" = $'check repo merge\ncheck repo merge' ]
  grep -q -- "--match-head-commit $HEAD_OID" "$GH_FIXTURES/merged"
}
