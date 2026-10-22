#!/usr/bin/env bats
setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup
}

refuse_override() {
  run env "$1=" bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"merge authority refuses environment variable $1"* ]]
}

@test "merge refuses DEVTOOLS_MERGE_OWNER" { refuse_override DEVTOOLS_MERGE_OWNER; }
@test "merge refuses DEVTOOLS_DELEGATION_CHECK" { refuse_override DEVTOOLS_DELEGATION_CHECK; }
@test "merge refuses DEVTOOLS_REQUIRED_REVIEWER" { refuse_override DEVTOOLS_REQUIRED_REVIEWER; }
@test "merge refuses DEVTOOLS_REQUIRED_REVIEWER_TYPE" { refuse_override DEVTOOLS_REQUIRED_REVIEWER_TYPE; }
@test "merge refuses DEVTOOLS_GH_LOGIN" { refuse_override DEVTOOLS_GH_LOGIN; }

# Commit a divergent checkout without advancing the PR's pinned base.
local_change() {
  git add -- "$1"
  git commit --quiet -m 'test: divergent checkout'
}

@test "local-package gates ignore clean divergent golden patterns" {
  change golden/result.txt protected
  sed -i 's/golden_paths = .*/golden_paths = ^$/' .agents/agent-skills.conf
  local_change .agents/agent-skills.conf
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"configuration differs from pinned base"* ]]
  [[ "$output" == *needs-owner* ]]
}

@test "local-package squash lint ignores a clean divergent command" {
  sed -i 's/lint_squash_cmd = .*/lint_squash_cmd = true/' .agents/agent-skills.conf
  local_change .agents/agent-skills.conf
  MISE_LINT_STATUS=1 run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"configuration differs from pinned base"* ]]
  grep -q 'run lint-squash-msg' "$GH_FIXTURES/mise.log"
}

remote_base() {
  git checkout --quiet main
  remote_lock
  local_change apm.lock.yaml
  BASE_OID=$(git rev-parse HEAD)
  base_at "$BASE_OID"
  git checkout --quiet topic
  git merge --quiet --no-edit main
  HEAD_OID=$(git rev-parse HEAD)
  pr ".headRefOid=\"$HEAD_OID\" | .reviews[0].commit.oid=\"$HEAD_OID\""
  git push --quiet origin main topic
}

@test "remote-package gates ignore clean divergent golden patterns" {
  remote_base
  change golden/result.txt protected
  sed -i 's/golden_paths = .*/golden_paths = ^$/' .agents/agent-skills.conf
  local_change .agents/agent-skills.conf
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"configuration differs from pinned base"* ]]
  [[ "$output" == *needs-owner* ]]
}

@test "remote-package squash lint ignores a clean divergent command" {
  remote_base
  sed -i 's/lint_squash_cmd = .*/lint_squash_cmd = true/' .agents/agent-skills.conf
  local_change .agents/agent-skills.conf
  MISE_LINT_STATUS=1 run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"configuration differs from pinned base"* ]]
  grep -q 'run lint-squash-msg' "$GH_FIXTURES/mise.log"
}

integrity_change() {
  printf '\n# divergent\n' >>"$1"
  local_change "$1"
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"merge integrity:"* ]]
  [[ "$output" != *"requires a clean tree"* ]]
}

@test "remote integrity refuses a committed policy edit" {
  remote_base
  integrity_change .claude/skills/merge-pr/scripts/merge-policy.py
}

@test "preflight refuses a verifier that accepts every checkout before sourcing helpers" {
  sed -i '/^def integrity(base: str, scripts: str) -> None:/a\    return' "$POLICY"
  printf '\nprintf "untrusted helper sourced\\n" >"$GH_FIXTURES/untrusted"\n' >>"$SCRIPTS/signing.sh"
  git add -- .claude/skills/merge-pr/scripts/merge-policy.py .claude/skills/merge-pr/scripts/signing.sh
  git commit --quiet -m 'test: bypass verifier'
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"merge integrity:"* ]]
  [[ "$output" != *"requires a clean tree"* ]]
  [ ! -f "$GH_FIXTURES/untrusted" ]
}

@test "remote integrity refuses a committed pull-request helper edit" {
  remote_base
  integrity_change .claude/skills/merge-pr/scripts/pull-request.sh
}

@test "remote integrity refuses a committed signing helper edit" {
  remote_base
  integrity_change .claude/skills/merge-pr/scripts/signing.sh
}

@test "local integrity refuses merge-pr edits when started from agent-merge-pr" {
  integrity_change .claude/skills/merge-pr/scripts/review.sh
}

@test "integrity refuses an extra merge-pr file" {
  integrity_change .claude/skills/merge-pr/unlisted
}

@test "integrity refuses an extra agent-merge-pr file" {
  integrity_change .claude/skills/agent-merge-pr/unlisted
}

@test "remote integrity accepts unmodified sibling skills" {
  remote_base
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -eq 0 ]
}

@test "local integrity accepts unmodified sibling skills" {
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -eq 0 ]
}

@test "final check refuses a verifier bypass and helper swap after message review" {
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -eq 0 ]
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  cat >"$BATS_TEST_TMPDIR/checker" <<'CHECK'
#!/usr/bin/env bash
sed -i '/^def integrity(base: str, scripts: str) -> None:/a\    return' .claude/skills/merge-pr/scripts/merge-policy.py
printf '\n# swapped after preflight\n' >>.claude/skills/merge-pr/scripts/signing.sh
git add -- .claude/skills/merge-pr/scripts/merge-policy.py .claude/skills/merge-pr/scripts/signing.sh
git commit --quiet -m 'test: swap helper'
exit 0
CHECK
  chmod +x "$BATS_TEST_TMPDIR/checker"
  machine delegation_check "$BATS_TEST_TMPDIR/checker"
  run bash "$AGENT/merge.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"merge integrity:"* ]]
  [[ "$output" != *"requires a clean tree"* ]]
  [ ! -f "$GH_FIXTURES/merged" ]
}

@test "a re-pin merges from the base checkout with its trusted deployment" {
  remote_base
  printf '\n# next deployment\n' >>.claude/skills/merge-pr/scripts/signing.sh
  remote_lock
  git add -- apm.lock.yaml .claude/skills/merge-pr/scripts/signing.sh
  git commit --quiet -m 'test: re-pin skill'
  HEAD_OID=$(git rev-parse HEAD)
  git push --quiet origin topic
  pr ".headRefOid=\"$HEAD_OID\" | .reviews[0].commit.oid=\"$HEAD_OID\""
  approve_owner
  git checkout --quiet main
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  grant merge
  run bash "$AGENT/merge.sh" 7
  [ "$status" -eq 0 ]
  [ -f "$GH_FIXTURES/merged" ]
}
