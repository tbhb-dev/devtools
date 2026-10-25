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
  sed 's/golden_paths = .*/golden_paths = ^$/' .agents/agent-skills.conf >.agents/agent-skills.conf.new
  mv .agents/agent-skills.conf.new .agents/agent-skills.conf
  local_change .agents/agent-skills.conf
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"configuration differs from pinned base"* ]]
  [[ "$output" == *needs-owner* ]]
}

@test "local-package squash lint ignores a clean divergent command" {
  sed 's/lint_squash_cmd = .*/lint_squash_cmd = true/' .agents/agent-skills.conf >.agents/agent-skills.conf.new
  mv .agents/agent-skills.conf.new .agents/agent-skills.conf
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
  sed 's/golden_paths = .*/golden_paths = ^$/' .agents/agent-skills.conf >.agents/agent-skills.conf.new
  mv .agents/agent-skills.conf.new .agents/agent-skills.conf
  local_change .agents/agent-skills.conf
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"configuration differs from pinned base"* ]]
  [[ "$output" == *needs-owner* ]]
}

@test "remote-package squash lint ignores a clean divergent command" {
  remote_base
  sed 's/lint_squash_cmd = .*/lint_squash_cmd = true/' .agents/agent-skills.conf >.agents/agent-skills.conf.new
  mv .agents/agent-skills.conf.new .agents/agent-skills.conf
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
  sed '/^def integrity(base: str, scripts: str) -> None:/a\
    __import__("pathlib").Path(__import__("os").environ["GH_FIXTURES"], "untrusted-reader").touch()\
    return
' "$POLICY" >"$POLICY.new"
  mv "$POLICY.new" "$POLICY"
  printf '\nprintf "untrusted helper sourced\\n" >"$GH_FIXTURES/untrusted"\n' >>"$SCRIPTS/signing.sh"
  git add -- .claude/skills/merge-pr/scripts/merge-policy.py .claude/skills/merge-pr/scripts/signing.sh
  git commit --quiet -m 'test: bypass verifier'
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"merge integrity:"* ]]
  [[ "$output" != *"requires a clean tree"* ]]
  [ ! -f "$GH_FIXTURES/untrusted" ]
  [ ! -f "$GH_FIXTURES/untrusted-reader" ]
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
sed '/^def integrity(base: str, scripts: str) -> None:/a\
    return
' .claude/skills/merge-pr/scripts/merge-policy.py >.claude/skills/merge-pr/scripts/merge-policy.py.new
mv .claude/skills/merge-pr/scripts/merge-policy.py.new .claude/skills/merge-pr/scripts/merge-policy.py
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

# Replace behavior, not just comments: the checkout dispatcher must not judge itself.
trust_attack() {
  export TRUST_ATTACK=$1
  cat >"$BATS_TEST_TMPDIR/checker" <<'CHECK'
#!/usr/bin/env bash
case $TRUST_ATTACK in
lib)
  sed '/^dt_merge_integrity() {/a\
  return 0
' .claude/skills/merge-pr/scripts/lib.sh >.claude/skills/merge-pr/scripts/lib.sh.new
  mv .claude/skills/merge-pr/scripts/lib.sh.new .claude/skills/merge-pr/scripts/lib.sh
  ;;
signing) ;;
extra) printf 'extra helper\n' >.claude/skills/agent-merge-pr/scripts/new-helper.sh ;;
esac
if [ "$TRUST_ATTACK" != extra ]; then
  printf '\nprintf "untrusted helper sourced\\n" >"$GH_FIXTURES/untrusted"\n' >>.claude/skills/merge-pr/scripts/signing.sh
fi
git add -- .claude/skills/merge-pr/scripts/lib.sh .claude/skills/merge-pr/scripts/signing.sh .claude/skills/agent-merge-pr/scripts
git commit --quiet -m 'test: replace merge helpers'
CHECK
  chmod +x "$BATS_TEST_TMPDIR/checker"
}

refuse_attack_preflight() {
  trust_attack "$1"
  bash "$BATS_TEST_TMPDIR/checker"
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"merge integrity:"* ]]
  [[ "$output" != *"requires a clean tree"* ]]
  [ ! -f "$GH_FIXTURES/untrusted" ]
  [ ! -f "$GH_FIXTURES/merged" ]
}

refuse_attack_final() {
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -eq 0 ]
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  trust_attack "$1"
  bash "$BATS_TEST_TMPDIR/checker"
  run bash "$AGENT/merge.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"merge integrity:"* ]]
  [[ "$output" != *"requires a clean tree"* ]]
  [ ! -f "$GH_FIXTURES/untrusted" ]
  [ ! -f "$GH_FIXTURES/merged" ]
}

@test "preflight refuses a disabled lib dispatcher before sourcing signing" { refuse_attack_preflight lib; }
@test "preflight refuses an executable signing edit" { refuse_attack_preflight signing; }
@test "preflight refuses a new helper file" { refuse_attack_preflight extra; }
@test "final check refuses a disabled lib dispatcher" { refuse_attack_final lib; }
@test "final check refuses an executable signing edit" { refuse_attack_final signing; }
@test "final check refuses a new helper file" { refuse_attack_final extra; }

refuse_attack_during_delegation() {
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  trust_attack "$1"
  machine delegation_check "$BATS_TEST_TMPDIR/checker"
  run bash "$AGENT/merge.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"merge integrity:"* ]]
  [ ! -f "$GH_FIXTURES/untrusted" ]
  [ ! -f "$GH_FIXTURES/merged" ]
}

@test "final gate refuses a lib dispatcher swapped during delegation" { refuse_attack_during_delegation lib; }
@test "final gate refuses signing swapped during delegation" { refuse_attack_during_delegation signing; }
@test "final gate refuses a helper added during delegation" { refuse_attack_during_delegation extra; }

@test "remote preflight refuses a disabled lib dispatcher" {
  remote_base
  refuse_attack_preflight lib
}

@test "merge-pr preflight refuses a disabled lib dispatcher directly" {
  trust_attack lib
  bash "$BATS_TEST_TMPDIR/checker"
  run env DEVTOOLS_UNATTENDED=1 bash "$SCRIPTS/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"merge integrity:"* ]]
  [ ! -f "$GH_FIXTURES/untrusted" ]
}

@test "unattended bootstrap reads pinned base blobs without archive extraction" {
  local entry
  export BOOTSTRAP_GIT
  BOOTSTRAP_GIT=$(command -v git)
  cat >"$BATS_TEST_TMPDIR/bin/git" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$GH_FIXTURES/bootstrap-git.log"
if [ "$1" = archive ]; then exit 1; fi
exec "$BOOTSTRAP_GIT" "$@"
STUB
  printf '#!/usr/bin/env bash\nexit 1\n' >"$BATS_TEST_TMPDIR/bin/tar"
  chmod +x "$BATS_TEST_TMPDIR/bin/git" "$BATS_TEST_TMPDIR/bin/tar"
  for entry in "$SCRIPTS" "$AGENT"; do
    : >"$GH_FIXTURES/bootstrap-git.log"
    run env DEVTOOLS_UNATTENDED=1 bash "$entry/preflight.sh" 7
    [ "$status" -eq 0 ]
    grep -Fx "cat-file blob $BASE_OID:.claude/skills/merge-pr/scripts/lib.sh" "$GH_FIXTURES/bootstrap-git.log"
    grep -Fx "cat-file blob $BASE_OID:.claude/skills/merge-pr/scripts/merge-policy.py" "$GH_FIXTURES/bootstrap-git.log"
  done
}

@test "successful unattended preflight removes its bootstrap snapshots" {
  local entry scratch=$BATS_TEST_TMPDIR/snapshots
  mkdir "$scratch"
  for entry in "$SCRIPTS" "$AGENT"; do
    run env TMPDIR="$scratch" DEVTOOLS_UNATTENDED=1 bash "$entry/preflight.sh" 7
    [ "$status" -eq 0 ]
    [ -z "$(find "$scratch" -name 'devtools-merge.*' -print)" ]
  done
}

@test "held unattended preflight removes its bootstrap snapshots and preserves status" {
  local entry scratch=$BATS_TEST_TMPDIR/snapshots
  mkdir "$scratch"
  pr '.reviewDecision="REVIEW_REQUIRED"'
  for entry in "$SCRIPTS" "$AGENT"; do
    run env TMPDIR="$scratch" DEVTOOLS_UNATTENDED=1 bash "$entry/preflight.sh" 7
    [ "$status" -eq 75 ]
    [[ "$output" == *WAITING_REVIEW* ]]
    [[ "$output" == *"held with status 75"* ]]
    [ -z "$(find "$scratch" -name 'devtools-merge.*' -print)" ]
  done
}
