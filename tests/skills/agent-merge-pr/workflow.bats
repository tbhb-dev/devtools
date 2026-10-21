#!/usr/bin/env bats
setup_file() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup_file
}

setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup
}

@test "unattended review requests every shared gate field" {
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
}

@test "unattended merger rejects confirmed even with session grant" {
  grant merge
  run bash "$AGENT/merge.sh" --confirmed 7
  [ "$status" -ne 0 ]
  [[ "$output" == *refuse*--confirmed* ]]
  [ ! -f "$GH_FIXTURES/merged" ]
}

@test "unattended merge holds without authority then merges with session grant" {
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  run bash "$AGENT/merge.sh" 7
  [ "$status" -eq 75 ]
  [ ! -f "$GH_FIXTURES/merged" ]
  grant merge
  run bash "$AGENT/merge.sh" 7
  [ "$status" -eq 0 ]
  [ -f "$GH_FIXTURES/merged" ]
  [[ "$output" == *"Current or non-agent worktree retained"* ]]
}

@test "machine identity and exact head reviewer approval are mandatory" {
  unset DEVTOOLS_GH_LOGIN
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *gh_login* ]]
  export DEVTOOLS_GH_LOGIN=other
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"signed in as"* ]]
  export DEVTOOLS_GH_LOGIN=agent
  pr '.reviews[0].commit.oid="old"'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *WAITING_REVIEW* ]]
}

@test "no checks skipped required checks and missing requirements refuse" {
  pr '.statusCheckRollup=[]'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  pr '.statusCheckRollup=[{__typename:"CheckRun",name:"test",status:"COMPLETED",conclusion:"SKIPPED"}]'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  echo '[]' >"$GH_FIXTURES/api/repos_{owner}_{repo}_rules_branches_main.json"
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
}

@test "base ruleset requirements survive PR edits and apply only to matching refs" {
  git checkout --quiet main
  mkdir -p .github/rulesets
  cat >.github/rulesets/main.json <<'RULE'
{"target":"branch","enforcement":"active","conditions":{"ref_name":{"include":["~DEFAULT_BRANCH"],"exclude":[]}},"rules":[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"base-check"}]}}]}
RULE
  git add -- .github/rulesets/main.json
  git commit --quiet -m 'test: base checks'
  BASE_OID=$(git rev-parse HEAD)
  run bash -c 'echo "[]" | uv run --quiet --script "$1" required "$2" main main .github/rulesets' _ "$POLICY" "$BASE_OID"
  [ "$status" -eq 0 ]
  [[ "$output" == *base-check* ]]
  run bash -c 'echo "[]" | uv run --quiet --script "$1" required "$2" other main .github/rulesets' _ "$POLICY" "$BASE_OID"
  [ "$status" -ne 0 ]
}

@test "readiness command refusal and changed message prevent a merge" {
  cat >"$BATS_TEST_TMPDIR/ready" <<'READY'
#!/usr/bin/env bash
exit 1
READY
  chmod +x "$BATS_TEST_TMPDIR/ready"
  printf '\nmerge_ready_cmd = %s\n' "$BATS_TEST_TMPDIR/ready" >>.agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: readiness'
  HEAD_OID=$(git rev-parse HEAD)
  git push --quiet origin topic
  pr ".headRefOid=\"$HEAD_OID\" | .reviews[0].commit.oid=\"$HEAD_OID\""
  approve_owner
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [ ! -f "$GH_FIXTURES/merged" ]
}

@test "owner approval withdrawn by delegation blocks before merge" {
  change golden/result.txt baseline
  approve_owner
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  cat >"$BATS_TEST_TMPDIR/checker" <<'CHECK'
#!/usr/bin/env bash
jq '.reviews[1].state="DISMISSED"' "$GH_FIXTURES/pr.json" >"$GH_FIXTURES/next.json"
mv "$GH_FIXTURES/next.json" "$GH_FIXTURES/pr.json"
exit 0
CHECK
  chmod +x "$BATS_TEST_TMPDIR/checker"
  export DEVTOOLS_DELEGATION_CHECK="$BATS_TEST_TMPDIR/checker"
  run bash "$AGENT/merge.sh" 7
  [ "$status" -ne 0 ]
  [ ! -f "$GH_FIXTURES/merged" ]
}

@test "remote message mismatch preserves drafts stamps and branches" {
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  grant merge
  export WRONG_MERGE_MESSAGE=1
  run bash "$AGENT/merge.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *merged-not-as-reviewed* ]]
  [ -f SQUASH_AGENTMSG ]
  [ -f "$(stamp_file)" ]
  git show-ref --verify refs/heads/topic
}

@test "persistent review finding bound survives preflight" {
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo "VERDICT: CHANGES REQUIRED"'
  for round in 1 2 3; do
    printf 'fix: explain the squash round %s (#7)\n' "$round" >SQUASH_AGENTMSG
    run bash "$AGENT/review.sh" 7
    if [ "$round" -eq 3 ]; then [ "$status" -eq 3 ]; else [ "$status" -eq 2 ]; fi
  done
  run bash "$AGENT/preflight.sh" 7
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 3 ]
}

@test "agent merge gives a new head fresh finding rounds" {
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo "VERDICT: CHANGES REQUIRED"'
  for round in 1 2 3; do
    printf 'fix: explain the squash round %s (#7)\n' "$round" >SQUASH_AGENTMSG
    run bash "$AGENT/review.sh" 7
  done
  [ "$status" -eq 3 ]

  change another.txt advanced
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 2 ]
}

@test "expected head mismatch and empty configured lint refuse" {
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  run bash "$AGENT/merge.sh" 7 1111111111111111111111111111111111111111
  [ "$status" -ne 0 ]
  [[ "$output" == *"Expected head differs"* ]]
  printf '\nlint_squash_cmd =\n' >>.agents/agent-skills.conf
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
}

@test "owner paths reach delegation as a restrictive signal" {
  change owned/input.txt content
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  cat >"$BATS_TEST_TMPDIR/checker" <<'CHECK'
#!/usr/bin/env bash
printf '%s\n' "$*" >"$GH_FIXTURES/delegation"
exit 1
CHECK
  chmod +x "$BATS_TEST_TMPDIR/checker"
  export DEVTOOLS_DELEGATION_CHECK="$BATS_TEST_TMPDIR/checker"
  run bash "$AGENT/merge.sh" 7
  [ "$status" -ne 0 ]
  grep -q -- '--owner-path' "$GH_FIXTURES/delegation"
}
