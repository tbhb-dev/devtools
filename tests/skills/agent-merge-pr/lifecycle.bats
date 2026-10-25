#!/usr/bin/env bats
setup_file() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup_file
}

setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup
}

@test "cleanup retains valuable ignored files and removes only disposable agent worktrees" {
  local work="$REPO/.claude/worktrees/child"
  git checkout --quiet main
  git worktree add --quiet "$work" topic
  echo '.env' >>.git/info/exclude
  echo secret >"$work/.env"
  run uv run --quiet --script "$POLICY" cleanup main topic "$HEAD_OID" "$HEAD_OID" '^cache/'
  [ "$status" -eq 0 ]
  [ -d "$work" ]
  [[ "$output" == *"valuable ignored files retained"* ]]
  rm "$work/.env"
  run uv run --quiet --script "$POLICY" cleanup main topic "$HEAD_OID" "$HEAD_OID" '^cache/'
  [ "$status" -eq 0 ]
  [ ! -d "$work" ]
}

@test "report transport failure does not repeat a completed merge" {
  cat >"$BATS_TEST_TMPDIR/mail" <<'MAIL'
#!/usr/bin/env bash
exit 1
MAIL
  chmod +x "$BATS_TEST_TMPDIR/mail"
  printf '\nmail_cmd = %s\nmail_to = recipient\n' "$BATS_TEST_TMPDIR/mail" >>.agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: reporting'
  HEAD_OID=$(git rev-parse HEAD)
  git push --quiet origin topic
  pr ".headRefOid=\"$HEAD_OID\" | .reviews[0].commit.oid=\"$HEAD_OID\""
  approve_owner
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  grant merge
  run bash "$AGENT/merge.sh" 7
  [ "$status" -eq 0 ]
  [[ "$output" == *"REPORT NOT SENT"* ]]
  [ "$(grep -c '^pr merge ' "$GH_FIXTURES/gh.log")" -eq 1 ]
}

@test "dirty trees interrupted operations and owner label hold unattended review" {
  echo uncommitted >new.txt
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"clean tree"* ]]
  rm new.txt
  touch "$(git rev-parse --absolute-git-dir)/MERGE_HEAD"
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *MERGE_HEAD* ]]
  rm "$(git rev-parse --absolute-git-dir)/MERGE_HEAD"
  pr '.labels=[{name:"owner-review"}]'
  approve_owner
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *owner-review* ]]
}

@test "remote cleanup leases deletion so a newer push survives" {
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  export ADVANCE_REMOTE_AFTER_MERGE=1
  grant merge
  run bash "$AGENT/merge.sh" 7
  [ "$status" -eq 0 ]
  [ "$(git -C "$GH_FIXTURES/upstream.git" rev-parse topic)" != "$HEAD_OID" ]
}

@test "dependent PRs retarget before merge and fork heads skip them" {
  echo '[{"number":8}]' >"$GH_FIXTURES/children.json"
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  grant merge
  run bash "$AGENT/merge.sh" 7
  [ "$status" -eq 0 ]
  grep -q 'pr edit 8 --base main' "$GH_FIXTURES/gh.log"
  grep -qx 'pr edit 8 --base main' "$GH_FIXTURES/gh-post.log"
}

@test "merge reviewer receives local commit bodies rather than capped API metadata" {
  pr '.commits[0].messageBody="invented API body"'
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; if grep -q "invented API body" "$DEVTOOLS_REVIEW_INPUTS/commits.txt"; then echo "VERDICT: CHANGES REQUIRED"; else echo "VERDICT: PASS"; fi'
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
}

@test "last readiness call cannot withdraw owner approval before merging" {
  cat >"$BATS_TEST_TMPDIR/ready" <<'READY'
#!/usr/bin/env bash
[ "${INVALIDATE_READY:-0}" = 1 ] || exit 0
count=$(cat "$GH_FIXTURES/ready-count" 2>/dev/null || echo 0)
count=$((count+1))
echo "$count" >"$GH_FIXTURES/ready-count"
if [ "$count" -eq 3 ]; then
  jq '.reviews[1].state="DISMISSED"' "$GH_FIXTURES/pr.json" >"$GH_FIXTURES/next.json"
  mv "$GH_FIXTURES/next.json" "$GH_FIXTURES/pr.json"
fi
READY
  chmod +x "$BATS_TEST_TMPDIR/ready"
  printf '\nmerge_ready_cmd = %s\n' "$BATS_TEST_TMPDIR/ready" >>.agents/agent-skills.conf
  echo protected >constitution.txt
  git add -- .agents/agent-skills.conf constitution.txt
  git commit --quiet -m 'test: readiness drift'
  HEAD_OID=$(git rev-parse HEAD)
  git push --quiet origin topic
  pr ".headRefOid=\"$HEAD_OID\" | .reviews[0].commit.oid=\"$HEAD_OID\""
  approve_owner
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  grant merge
  export INVALIDATE_READY=1
  run bash "$AGENT/merge.sh" 7
  [ "$status" -ne 0 ]
  [ ! -f "$GH_FIXTURES/merged" ]
}

@test "main fast forward preserves valuable ignored files" {
  printf 'tracked replacement\n' >.env
  git add -f -- .env
  git commit --quiet -m 'test: tracked input'
  HEAD_OID=$(git rev-parse HEAD)
  git checkout --quiet main
  echo '.env' >>.git/info/exclude
  echo valuable >.env
  run uv run --quiet --script "$POLICY" cleanup main topic "$HEAD_OID" "$HEAD_OID" '^cache/'
  [ "$status" -eq 0 ]
  [ "$(cat .env)" = valuable ]
  git show-ref --verify refs/heads/topic
}

@test "readiness cannot replace the reviewed merge body" {
  cat >"$BATS_TEST_TMPDIR/ready" <<'READY'
#!/usr/bin/env bash
state=$(git rev-parse --absolute-git-dir)/agent-skills
printf 'unreviewed replacement\n' >"$state/merge.body"
READY
  chmod +x "$BATS_TEST_TMPDIR/ready"
  printf '\nmerge_ready_cmd = %s\n' "$BATS_TEST_TMPDIR/ready" >>.agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: readiness message drift'
  HEAD_OID=$(git rev-parse HEAD)
  git push --quiet origin topic
  pr ".headRefOid=\"$HEAD_OID\" | .reviews[0].commit.oid=\"$HEAD_OID\""
  approve_owner
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  grant merge
  run bash "$AGENT/merge.sh" 7
  [ "$status" -eq 0 ]
  grep -q 'The thing was missing' "$GH_FIXTURES/message"
}

@test "merge holds use the shared blocker transport and keep the failure status" {
  cat >"$BATS_TEST_TMPDIR/mail" <<'MAIL'
#!/usr/bin/env bash
printf '%s\n' "$*" >"$GH_FIXTURES/mail-report"
MAIL
  chmod +x "$BATS_TEST_TMPDIR/mail"
  printf '\nmail_cmd = %s\nmail_to = recipient\n' "$BATS_TEST_TMPDIR/mail" >>.agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: hold reporting'
  run bash "$AGENT/merge.sh" 7
  [ "$status" -ne 0 ]
  grep -q -- '--kind blocker' "$GH_FIXTURES/mail-report"
  [ ! -f "$GH_FIXTURES/merged" ]
}
