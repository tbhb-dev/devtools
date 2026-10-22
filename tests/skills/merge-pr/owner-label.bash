# Shared acceptance cases for the merge-pr and agent-merge-pr entry points.
owner_label_setup() {
  source "${BATS_TEST_DIRNAME}/../agent-merge-pr/test_helper.bash"
  agent_setup
  if [ "$1" = agent ]; then
    LABEL_REVIEW=$AGENT/review.sh
    LABEL_MERGE=$AGENT/merge.sh
  else
    LABEL_REVIEW=$SCRIPTS/review.sh
    LABEL_MERGE=$SCRIPTS/squash-merge.sh
    export MA_REQUIRE_GATES=true
  fi
  pr '.labels=[{name:"owner-review"}]'
}

owner_label_hold() {
  run bash "$LABEL_REVIEW" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"$1"* ]] || { printf '%s\n' "$output"; return 1; }
  [ ! -f "$GH_FIXTURES/merged" ]
}

owner_label_release() {
  approve_owner
  # Comments don't supersede the latest decisive owner review.
  pr ".reviews += [{author:{login:\"owner\"},state:\"COMMENTED\",commit:{oid:\"$HEAD_OID\"},submittedAt:\"2026-01-04T00:00:00Z\"}]"
  bash "$LABEL_REVIEW" 7
  grant merge
  run bash "$LABEL_MERGE" 7
  [ "$status" -eq 0 ]
  [ -f "$GH_FIXTURES/merged" ]
  [[ "$output" == *"Owner-review label released by owner at $HEAD_OID."* ]]
  if [ "$LABEL_MERGE" = "$AGENT/merge.sh" ]; then
    jq -e --arg head "$HEAD_OID" '.owner_label_release == {login:"owner",head:$head} and .owner_override == ""' "$(git rev-parse --absolute-git-dir)/agent-skills/merge.result.json"
  fi
}

owner_label_old_head() {
  approve_owner
  pr '.reviews[1].commit.oid="old"'
  owner_label_hold 'owner approved an older head'
}

owner_label_later_review() {
  approve_owner
  pr ".reviews += [{author:{login:\"owner\"},state:\"$1\",commit:{oid:\"$HEAD_OID\"},submittedAt:\"2026-01-03T00:00:00Z\"}]"
  owner_label_hold "latest review from owner is $1"
}

owner_label_no_owner() {
  approve_owner
  unset DEVTOOLS_MERGE_OWNER
  owner_label_hold 'no merge_owner configured'
}

owner_label_other_reviewer() {
  export DEVTOOLS_MERGE_OWNER=owner
  if [ "$1" = other ]; then
    pr ".reviews += [{author:{login:\"other\"},state:\"APPROVED\",commit:{oid:\"$HEAD_OID\"},submittedAt:\"2026-01-02T00:00:00Z\"}]"
  fi
  owner_label_hold 'owner has no decisive review'
}

owner_label_missing_reviewer() {
  approve_owner
  pr '.reviews |= map(select(.author.login != "reviewer"))'
  owner_label_hold 'WAITING_REVIEW: reviewer must approve the current head'
}

owner_label_failed_check() {
  approve_owner
  pr '.statusCheckRollup[0].conclusion="FAILURE"'
  owner_label_hold '"outcome":1'
}

owner_label_final_drift() {
  # Mutate the live snapshot after the last readiness call, so only the
  # final read and gate can catch a same-head owner review withdrawal.
  cat >"$BATS_TEST_TMPDIR/ready" <<'READY'
#!/usr/bin/env bash
[ -n "${LABEL_DRIFT:-}" ] || exit 0
count=$(cat "$GH_FIXTURES/ready-count" 2>/dev/null || echo 0)
count=$((count+1))
echo "$count" >"$GH_FIXTURES/ready-count"
if [ "$count" -eq 3 ]; then
  if [ "$LABEL_DRIFT" = head ]; then
    jq '.headRefOid="new-head"' "$GH_FIXTURES/pr.json" >"$GH_FIXTURES/next.json"
  else
    jq '.reviews += [{author:{login:"owner"},state:"CHANGES_REQUESTED",commit:{oid:.headRefOid},submittedAt:"2026-01-03T00:00:00Z"}]' "$GH_FIXTURES/pr.json" >"$GH_FIXTURES/next.json"
  fi
  mv "$GH_FIXTURES/next.json" "$GH_FIXTURES/pr.json"
fi
READY
  chmod +x "$BATS_TEST_TMPDIR/ready"
  printf '\nmerge_ready_cmd = %s\n' "$BATS_TEST_TMPDIR/ready" >>.agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: label approval drift'
  HEAD_OID=$(git rev-parse HEAD)
  git push --quiet origin topic
  pr ".headRefOid=\"$HEAD_OID\" | .reviews[0].commit.oid=\"$HEAD_OID\""
  approve_owner
  bash "$LABEL_REVIEW" 7
  grant merge
  export LABEL_DRIFT=$1
  run bash "$LABEL_MERGE" 7
  [ "$status" -ne 0 ]
  [ "$(cat "$GH_FIXTURES/ready-count")" -eq 3 ]
  if [ "$1" = head ]; then
    [[ "$output" == *"Review again against the current draft and branch"* ]]
  else
    [[ "$output" == *"latest review from owner is CHANGES_REQUESTED"* ]]
  fi
  [ ! -f "$GH_FIXTURES/merged" ]
}
