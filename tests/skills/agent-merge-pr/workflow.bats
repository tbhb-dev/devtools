#!/usr/bin/env bats
setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup
}

@test "unattended review requests every shared gate field" {
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
}

@test "unattended merge rejects an unusable check runner" {
  export REAL_AGENTREPO
  REAL_AGENTREPO=$(command -v agentrepo)
  mkdir -p "$BATS_TEST_TMPDIR/broken-bin"
  printf '#!/usr/bin/env bash\nif [ "$1" = config ] || [ "$1" = --version ]; then exec "$REAL_AGENTREPO" "$@"; fi\nexit 1\n' >"$BATS_TEST_TMPDIR/broken-bin/agentrepo"
  chmod +x "$BATS_TEST_TMPDIR/broken-bin/agentrepo"

  PATH="$BATS_TEST_TMPDIR/broken-bin:$PATH" run bash "$AGENT/preflight.sh" 7

  [ "$status" -ne 0 ]
  [[ "$output" == *"a working agentrepo is required for check judgement"* ]]
}

@test "unattended merge evaluates checks through the pinned agentrepo" {
  mkdir -p "$BATS_TEST_TMPDIR/stale-bin" "$BATS_TEST_TMPDIR/pinned-bin"
  printf '#!/bin/sh\nif [ "$1" = --version ]; then echo "agentrepo version v0.14.2"; exit 0; fi\necho "$*" >>"$BATS_TEST_TMPDIR/stale-calls"\nexit 1\n' >"$BATS_TEST_TMPDIR/stale-bin/agentrepo"
  printf '#!/bin/sh\necho "$*" >>"$BATS_TEST_TMPDIR/pinned-calls"\nexec "%s" "$@"\n' "$BATS_SUITE_TMPDIR/agentrepo" >"$BATS_TEST_TMPDIR/pinned-bin/agentrepo"
  printf '#!/bin/sh\nif [ "$*" = "which agentrepo" ]; then echo "%s/agentrepo"; exit 0; fi\nexec "%s" "$@"\n' "$BATS_TEST_TMPDIR/pinned-bin" "$(command -v mise)" >"$BATS_TEST_TMPDIR/stale-bin/mise"
  chmod +x "$BATS_TEST_TMPDIR"/stale-bin/* "$BATS_TEST_TMPDIR"/pinned-bin/*

  PATH="$BATS_TEST_TMPDIR/stale-bin:$PATH" run bash "$AGENT/review.sh" 7

  [ "$status" -eq 0 ]
  grep -q '^checks evaluate ' "$BATS_TEST_TMPDIR/pinned-calls"
  grep -q '^config merge-machine -- ' "$BATS_TEST_TMPDIR/pinned-calls"
  grep -q '^config repository --base ' "$BATS_TEST_TMPDIR/pinned-calls"
  [ ! -e "$BATS_TEST_TMPDIR/stale-calls" ]
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
  machine gh_login ""
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *gh_login* ]]
  machine gh_login other
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"signed in as"* ]]
  machine gh_login agent
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
  git checkout --quiet main
  printf '\nmerge_ready_cmd = %s\n' "$BATS_TEST_TMPDIR/ready" >>.agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: readiness'
  BASE_OID=$(git rev-parse HEAD)
  base_at "$BASE_OID"
  git checkout --quiet topic
  git merge --quiet --no-edit main
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
  machine delegation_check "$BATS_TEST_TMPDIR/checker"
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
  machine delegation_check "$BATS_TEST_TMPDIR/checker"
  run bash "$AGENT/merge.sh" 7
  [ "$status" -ne 0 ]
  grep -q -- '--owner-path' "$GH_FIXTURES/delegation"
}

advance_base() {
  git checkout --quiet main
  printf 'newer\n' >newer.txt
  git add -- newer.txt
  git commit --quiet -m 'test: advance main'
  BASE_OID=$(git rev-parse HEAD)
  git push --quiet origin main
  git checkout --quiet topic
  base_at "$BASE_OID"
}

update_branch() {
  git rebase --quiet main
  HEAD_OID=$(git rev-parse HEAD)
  git push --quiet --force-with-lease="refs/heads/topic:$(git -C "$GH_FIXTURES/upstream.git" rev-parse topic)" origin topic
  pr ".headRefOid=\"$HEAD_OID\" | .reviews[0].commit.oid=\"$HEAD_OID\""
}

@test "skipped required check on a lagging merge base fails with the update-branch hint" {
  advance_base
  pr '.statusCheckRollup=[{__typename:"CheckRun",name:"test",status:"COMPLETED",conclusion:"SKIPPED"}]'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"skipped required check: test"* ]]
  [[ "$output" == *"merge base differs from base commit: yes"* ]]
  [[ "$output" == *"run: gh pr update-branch 7"* ]]
}

@test "skipped required check on a current merge base still fails and reports it" {
  pr '.statusCheckRollup=[{__typename:"CheckRun",name:"test",status:"COMPLETED",conclusion:"SKIPPED"}]'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"skipped required check: test"* ]]
  [[ "$output" == *"merge base differs from base commit: no"* ]]
  [[ "$output" == *"run: gh pr update-branch 7"* ]]
}

@test "skipped required check beside a missing required check fails with the update-branch hint" {
  advance_base
  echo '[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"validate"},{"context":"test"}]}}]' >"$GH_FIXTURES/api/repos_{owner}_{repo}_rules_branches_main.json"
  pr '.statusCheckRollup=[{__typename:"CheckRun",name:"validate",status:"COMPLETED",conclusion:"SKIPPED"}]'
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 3 ]
  [[ "$output" == *"skipped required check: validate"* ]]
  [[ "$output" == *"merge base differs from base commit: yes"* ]]
  [[ "$output" == *"run: gh pr update-branch 7"* ]]
}

@test "update-branch with passing required checks clears the gate and the hint" {
  advance_base
  pr '.statusCheckRollup=[{__typename:"CheckRun",name:"test",status:"COMPLETED",conclusion:"SKIPPED"}]'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  update_branch
  pr '.statusCheckRollup=[{__typename:"CheckRun",name:"test",status:"COMPLETED",conclusion:"SUCCESS"}]'
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  [[ "$output" != *"update-branch"* ]]
  [[ "$output" != *"skipped required check"* ]]
}

@test "a neutral required check is not waived and gets no stale-ref hint" {
  advance_base
  pr '.statusCheckRollup=[{__typename:"CheckRun",name:"test",status:"COMPLETED",conclusion:"NEUTRAL"}]'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" != *"update-branch"* ]]
}

@test "AC1 to AC9 squash redesign preserves the bound and rejected drafts" {
  source "${BATS_TEST_DIRNAME}/../redesign.bash"
  redesign_flow squash SQUASH_AGENTMSG "$AGENT/review.sh" 7
}
