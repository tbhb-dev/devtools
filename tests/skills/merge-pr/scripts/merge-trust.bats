#!/usr/bin/env bats
setup() {
  source "${BATS_TEST_DIRNAME}/../../agent-merge-pr/test_helper.bash"
  agent_setup
}

@test "merge-pr gates refuse authority overrides without an agent entry point" {
  for name in DEVTOOLS_MERGE_OWNER DEVTOOLS_DELEGATION_CHECK DEVTOOLS_REQUIRED_REVIEWER DEVTOOLS_REQUIRED_REVIEWER_TYPE DEVTOOLS_GH_LOGIN; do
    run env DEVTOOLS_UNATTENDED=1 "$name=override" bash "$SCRIPTS/preflight.sh" 7
    [ "$status" -ne 0 ]
    [[ "$output" == *"merge authority refuses environment variable $name"* ]]
  done
}

@test "the final gate repeats the whole sibling-directory integrity check" {
  run bash -c '
    source "$1/lib.sh"
    source "$1/pull-request.sh"
    DEVTOOLS_UNATTENDED=1
    ma_gates "$GH_FIXTURES/pr.json" 7 "$2" || exit $?
    printf "\n# replaced before final check\n" >>"$1/signing.sh"
    git add -- .claude/skills/merge-pr/scripts/signing.sh
    git commit --quiet -m "test: replace helper"
    ma_gates "$GH_FIXTURES/pr.json" 7 "$2" true
  ' bash "$SCRIPTS" "$BASE_OID"
  [ "$status" -ne 0 ]
  [[ "$output" == *"merge integrity:"* ]]
  [[ "$output" != *"requires a clean tree"* ]]
}

@test "merge fallback pins configuration and refuses environment authority" {
  # Model an older agentrepo that cannot provide strict config, but can judge checks.
  export REAL_AGENTREPO
  REAL_AGENTREPO=$(command -v agentrepo)
  cat >"$BATS_TEST_TMPDIR/bin/agentrepo" <<'STUB'
#!/usr/bin/env bash
if [ "$1" = config ]; then exit 1; fi
exec "$REAL_AGENTREPO" "$@"
STUB
  chmod +x "$BATS_TEST_TMPDIR/bin/agentrepo"
  PATH="$BATS_TEST_TMPDIR/bin:$PATH" run env DEVTOOLS_UNATTENDED=1 bash "$SCRIPTS/preflight.sh" 7
  [ "$status" -eq 0 ]
  PATH="$BATS_TEST_TMPDIR/bin:$PATH" run env DEVTOOLS_UNATTENDED=1 DEVTOOLS_GH_LOGIN=override bash "$SCRIPTS/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"refuses environment variable DEVTOOLS_GH_LOGIN"* ]]
  sed 's/golden_paths = .*/golden_paths = ^$/' .agents/agent-skills.conf >.agents/agent-skills.conf.new
  mv .agents/agent-skills.conf.new .agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: divergent configuration'
  change golden/result.txt protected
  PATH="$BATS_TEST_TMPDIR/bin:$PATH" run env DEVTOOLS_UNATTENDED=1 bash "$SCRIPTS/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"configuration differs from pinned base"* ]]
  [[ "$output" == *needs-owner* ]]
}
