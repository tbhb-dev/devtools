#!/usr/bin/env bats

setup() {
  bats_require_minimum_version 1.5.0
  source "${BATS_TEST_DIRNAME}/../../../test_helper.bash"
  isolate_git
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
  SCRIPTS="$ROOT/packages/agents/claude/.apm/skills/rebase/scripts"
  git init --quiet --initial-branch=main "$BATS_TEST_TMPDIR/repo"
  cd "$BATS_TEST_TMPDIR/repo"
  git config user.name Tester
  git config user.email tester@example.com
  printf 'one\n' >file
  git add -- file
  git commit --quiet -m 'test: initial state'
  git checkout --quiet -b topic
  printf 'topic\n' >file
  git commit --quiet -am 'test: topic edit'
  git checkout --quiet main
  printf 'main\n' >file
  git commit --quiet -am 'test: main edit'
  git checkout --quiet topic
  unset CLAUDE_CODE_SESSION_ID CODEX_THREAD_ID
  export XDG_CONFIG_HOME="$BATS_TEST_TMPDIR/config"
  export GIT_EDITOR=true REBASE_PREFLIGHT_FETCH=0
}

check_failure() {
  run --separate-stderr ! bash "$SCRIPTS/marker-scan.sh" file
  [ "$status" -ne 0 ]
  [[ "$output" == *"marker-scan.sh: agentrepo markers scan failed"* ]]
  [ -f "$BATS_TEST_TMPDIR/probed" ]

  run git rebase main
  [ "$status" -ne 0 ]
  run bash "$SCRIPTS/rebase-status.sh"
  [[ "$output" == *"marker-scan.sh: agentrepo markers scan failed"* ]]
  [[ "$output" == *"conflict markers in the worktree"* ]]

  printf 'resolved\n' >file
  git add -- file
  run bash "$SCRIPTS/continue-rebase.sh"
  [ "$status" -eq 2 ]
  [[ "$output" == *"marker-scan.sh: agentrepo markers scan failed"* ]]
  [[ "$output" == *"Continuing would commit them"* ]]
  git rebase --abort

  printf '<<<<<<< left\n' >file
  git commit --quiet -am 'test: committed marker'
  run bash "$SCRIPTS/verify-rebase.sh" HEAD
  [ "$status" -eq 1 ]
  [[ "$output" == *"marker-scan.sh: agentrepo markers scan failed"* ]]
  [[ "$output" == *"resolution that never finished"* ]]
}

scan_matches() {
  local expected_status=$1 expected_output=$2 expected_stderr=$3 actual_status=0
  shift 3
  bash "$SCRIPTS/marker-scan.sh" "$@" >actual 2>errors || actual_status=$?
  [ "$actual_status" -eq "$expected_status" ]
  printf '%s' "$expected_output" | cmp - actual
  printf '%s' "$expected_stderr" | cmp - errors
}

check_fallback() {
  printf '<<<<<<< left\n=======\n>>>>>>> right\n' >one
  printf '|||||||\n' >two
  printf 'clean\n' >clean
  mkdir -p skills/resolve-rebase-conflicts
  printf '<<<<<<<\n' >skills/resolve-rebase-conflicts/SKILL.md
  printf '<<<<<<<\n' >documented
  printf 'documented conflict-markers=documented\n' >.gitattributes
  git add -- one two clean documented .gitattributes skills

  # These reports come from the previous shell scanner's fixtures.
  scan_matches 0 '' '' clean
  scan_matches 1 $'one  2 marker line(s)\n' '' one
  scan_matches 1 $'two  1 marker line(s)\none  2 marker line(s)\n' '' two one
  scan_matches 1 $'one  2 marker line(s)\none  2 marker line(s)\n' '' one one
  scan_matches 0 '' '' '' missing documented skills/resolve-rebase-conflicts/SKILL.md
  scan_matches 1 $'one  2 marker line(s)\n' '' '' one ''
  scan_matches 2 '' $'usage: marker-scan.sh [--staged] <path> [path ...]\n'
  scan_matches 2 '' $'usage: marker-scan.sh [--staged] <path> [path ...]\n' --staged

  printf 'clean\n' >one
  printf '>>>>>>> worktree\n' >clean
  scan_matches 1 $'one  2 marker line(s)\n' '' --staged one clean missing
  scan_matches 1 $'clean  1 marker line(s)\n' '' one clean missing
}

@test "a missing Go scanner preserves the previous shell reports" {
  export PATH=/usr/bin:/bin
  ! command -v agentrepo
  check_fallback
}

@test "an older Go scanner preserves the previous shell reports" {
  mkdir "$BATS_TEST_TMPDIR/bin"
  cat >"$BATS_TEST_TMPDIR/bin/agentrepo" <<'STUB'
#!/usr/bin/env bash
printf 'unknown command "markers" for "agentrepo"\n' >&2
exit 5
STUB
  chmod +x "$BATS_TEST_TMPDIR/bin/agentrepo"
  export PATH="$BATS_TEST_TMPDIR/bin:/usr/bin:/bin"
  check_fallback
}

@test "a supported Go scanner receives the probe and scan arguments" {
  mkdir "$BATS_TEST_TMPDIR/bin"
  export RECORD="$BATS_TEST_TMPDIR/record"
  cat >"$BATS_TEST_TMPDIR/bin/agentrepo" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$@" >>"$RECORD"
[ "$#" -ne 3 ] || [ -n "$3" ] || exit 0
printf 'Go scanner report\n'
exit 1
STUB
  chmod +x "$BATS_TEST_TMPDIR/bin/agentrepo"
  export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
  scan_matches 1 $'Go scanner report\n' '' --staged file
  printf 'markers\nscan\n\nmarkers\nscan\n--staged\nfile\n' | cmp - "$RECORD"
}

@test "a Go scanner that exits without output reports failure through every caller" {
  mkdir "$BATS_TEST_TMPDIR/bin"
  cat >"$BATS_TEST_TMPDIR/bin/agentrepo" <<'STUB'
#!/usr/bin/env bash
if [ "$#" -eq 3 ] && [ -z "$3" ]; then
  touch "$BATS_TEST_TMPDIR/probed"
  exit 0
fi
exit 1
STUB
  chmod +x "$BATS_TEST_TMPDIR/bin/agentrepo"
  export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
  check_failure
}

@test "a scanner startup error stays on stdout even when it prints stderr" {
  mkdir "$BATS_TEST_TMPDIR/bin"
  cat >"$BATS_TEST_TMPDIR/bin/agentrepo" <<'STUB'
#!/usr/bin/env bash
[ "$#" -ne 3 ] || [ -n "$3" ] || exit 0
printf 'startup error\n' >&2
exit 2
STUB
  chmod +x "$BATS_TEST_TMPDIR/bin/agentrepo"
  PATH="$BATS_TEST_TMPDIR/bin:$PATH" run --separate-stderr bash "$SCRIPTS/marker-scan.sh" file
  [ "$status" -eq 2 ]
  [[ "$output" == *"marker-scan.sh: agentrepo markers scan failed"* ]]
  [ "$stderr" = 'startup error' ]
}

@test "migration note explains marker scanner compatibility" {
  local note
  note=$(cat "$ROOT/docs/agent-skills-migration.md")
  [[ "$note" == *'agentrepo markers scan'* ]]
  [[ "$note" == *'shell fallback'*'older binaries'* ]]
  [[ "$note" == *'issues/333'*'issues/544'*'N-2'* ]]
}
