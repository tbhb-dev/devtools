#!/usr/bin/env bats
# The rebase skill's scripts, as generated: the per-run state record that
# lets verify-rebase.sh range-diff the right ranges. Each case runs in a
# scratch repository with no global git configuration, and the two sides
# of the rebase conflict on one line so the replay stops.

setup() {
  source "${BATS_TEST_DIRNAME}/../../../test_helper.bash"
  isolate_git
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
  SCRIPTS="${ROOT}/packages/agents/claude/.apm/skills/rebase/scripts"
  REPO="${BATS_TEST_TMPDIR}/repo"
  unset CLAUDE_CODE_SESSION_ID CODEX_THREAD_ID
  export XDG_CONFIG_HOME="${BATS_TEST_TMPDIR}/config"
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
  export REBASE_PREFLIGHT_FETCH=0 GIT_EDITOR=true
  git init --quiet --initial-branch=main "$REPO"
  git -C "$REPO" config user.name Tester
  git -C "$REPO" config user.email tester@example.com
  cd "$REPO"
  printf 'one\n' >f.txt
  git add -- f.txt
  git commit --quiet -m 'test: initial state'
  git checkout --quiet -b topic
  printf 'topic\n' >f.txt
  git commit --quiet -am 'test: topic edit'
  git checkout --quiet main
  printf 'main\n' >f.txt
  git commit --quiet -am 'test: main edit'
  git checkout --quiet topic
}

state_file() { printf '%s/rebase-agent-state' "$(git rev-parse --absolute-git-dir)"; }

resolve_conflict() {
  printf 'both\n' >f.txt
  git add -- f.txt
}

@test "verify refuses the state an aborted rebase left behind" {
  run bash "$SCRIPTS/start-rebase.sh" main
  [ "$status" -eq 1 ]
  [ -f "$(state_file)" ]
  git rebase --abort

  run bash "$SCRIPTS/verify-rebase.sh"

  [ "$status" -eq 1 ]
  [[ "$output" == *"never marked complete"* ]]
  [[ "$output" != *"(state)"* ]]
  [ ! -e "$(state_file)" ]
}

@test "preflight clears the state an aborted rebase left behind" {
  run bash "$SCRIPTS/start-rebase.sh" main
  git rebase --abort

  run bash "$SCRIPTS/preflight.sh"

  [[ "$output" == *"cleared an incomplete state record"* ]]
  [ ! -e "$(state_file)" ]
}

@test "a rebase another workflow started still verifies against the old range" {
  run git rebase main
  [ "$status" -ne 0 ]
  before=$(cat "$(git rev-parse --absolute-git-dir)/rebase-merge/orig-head")

  run bash "$SCRIPTS/preflight.sh"
  [[ "$output" == *"reconstructed from the active rebase"* ]]
  grep -q "^before=$before\$" "$(state_file)"

  resolve_conflict
  run bash "$SCRIPTS/continue-rebase.sh"
  [ "$status" -eq 0 ]

  run bash "$SCRIPTS/verify-rebase.sh"
  [[ "$output" == *"comparing against"*"(state)"* ]]
}

@test "a finished replay marks the record complete and verify consumes it" {
  run bash "$SCRIPTS/start-rebase.sh" main
  [ "$status" -eq 1 ]
  resolve_conflict
  run bash "$SCRIPTS/continue-rebase.sh"
  [ "$status" -eq 0 ]
  [ -f "$(state_file).completed" ]

  run bash "$SCRIPTS/verify-rebase.sh"

  [[ "$output" == *"comparing against"*"(state)"* ]]
  [[ "$output" != *"never marked complete"* ]]
}

@test "starting a rebase discards a record from an earlier run" {
  printf 'before=stale\nbase_sha=stale\n' >"$(state_file)"
  : >"$(state_file).completed"
  git checkout --quiet main

  run bash "$SCRIPTS/start-rebase.sh" main

  [ ! -e "$(state_file)" ]
  [ ! -e "$(state_file).completed" ]
}

@test "preflight advises a throwaway wip commit for dirty work" {
  printf 'dirty\n' >untracked-change.txt
  git add -- untracked-change.txt

  run bash "$SCRIPTS/preflight.sh"

  [[ "$output" == *"throwaway wip commit"* ]]
}

@test "continue refuses staged paths printed before git fails" {
  run bash "$SCRIPTS/start-rebase.sh" main
  [ "$status" -eq 1 ]
  resolve_conflict
  export REAL_GIT
  REAL_GIT=$(command -v git)
  mkdir -p "$BATS_TEST_TMPDIR/bin"
  cat >"$BATS_TEST_TMPDIR/bin/git" <<'STUB'
#!/usr/bin/env bash
"$REAL_GIT" "$@" || exit $?
case " $* " in *' diff --no-ext-diff --cached --name-only '*) exit 9 ;; esac
STUB
  chmod +x "$BATS_TEST_TMPDIR/bin/git"
  PATH="$BATS_TEST_TMPDIR/bin:$PATH" run bash "$SCRIPTS/continue-rebase.sh"
  [ "$status" -eq 9 ]
  [ -d .git/rebase-merge ]
}
