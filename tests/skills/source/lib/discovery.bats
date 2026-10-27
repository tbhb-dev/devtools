#!/usr/bin/env bats

setup() {
  bats_require_minimum_version 1.5.0
  source "${BATS_TEST_DIRNAME}/../../../test_helper.bash"
  isolate_git
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
  export LIB="$ROOT/packages/agents/source/lib/lib.sh"
  git init --quiet --initial-branch=main "$BATS_TEST_TMPDIR/repo"
  cd "$BATS_TEST_TMPDIR/repo"
  export _DEVTOOLS_AGENTREPO_BIN="$BATS_TEST_TMPDIR/agentrepo"
  export _DEVTOOLS_AGENTREPO_CONFIG=false
  export _DEVTOOLS_AGENTREPO_VERDICT=false
  cat >"$_DEVTOOLS_AGENTREPO_BIN" <<'STUB'
#!/bin/sh
if [ "$*" = '--help' ]; then printf '  context  Discover skill context\n'; exit 0; fi
printf '%s' "${REPLY:-}"
exit "${RESULT:-0}"
STUB
  chmod +x "$_DEVTOOLS_AGENTREPO_BIN"
}

@test "AC3: a supported discovery failure refuses without running the shell" {
  export RESULT=9
  run --separate-stderr bash -c '. "$LIB"; gitr() { touch shell-ran; }; dt_root'
  [ "$status" -eq 9 ]
  [ -z "$output" ]
  [ -n "$stderr" ]
  [ ! -e shell-ran ]
}

@test "AC4: complete-looking failed and truncated successful output are refused" {
  for RESULT in 0 9; do
    export RESULT
    REPLY=$'devtools-context-v1\n/complete/looking/path'
    if [ "$RESULT" -ne 0 ]; then REPLY=$REPLY$'\ndevtools-context-end'; fi
    export REPLY
    run --separate-stderr bash -c '. "$LIB"; gitr() { touch shell-ran; }; dt_root'
    [ "$status" -ne 0 ]
    [ -z "$output" ]
    [ -n "$stderr" ]
    [[ "$stderr" != *"/complete/looking/path"* ]]
    [ ! -e shell-ran ]
  done
}

# Run the same sourcing context with and without the capability. All values
# are captured separately so diagnostics cannot masquerade as a result.
compare_paths() {
  local function=$1
  shift
  export _DEVTOOLS_AGENTREPO_CONTEXT=false
  run --separate-stderr bash -c '. "$LIB"; "$@"' _ "$function" "$@"
  local want_status=$status want_out=$output want_err=$stderr
  export _DEVTOOLS_AGENTREPO_CONTEXT=true
  run --separate-stderr bash -c '. "$LIB"; "$@"' _ "$function" "$@"
  [ "$status" -eq "$want_status" ]
  [ "$output" = "$want_out" ]
  [ "$stderr" = "$want_err" ]
}

@test "AC1 AC8: an older binary keeps the shell result without calling context" {
  cat >"$_DEVTOOLS_AGENTREPO_BIN" <<'STUB'
#!/bin/sh
[ "$1" != context ] || { touch context-ran; exit 99; }
exit 0
STUB
  run --separate-stderr bash -c '. "$LIB"; dt_root'
  [ "$status" -eq 0 ]
  [ "$output" = "$(git rev-parse --show-toplevel)" ]
  [ -z "$stderr" ]
  [ ! -e context-ran ]
}

@test "AC2 AC6: repository, unborn HEAD, linked worktree and paths match the shell" {
  export _DEVTOOLS_AGENTREPO_BIN="$(command -v agentrepo)"
  unset DEVTOOLS_HARNESS CLAUDE_CODE_SESSION_ID CLAUDECODE CODEX_THREAD_ID
  for function in dt_root dt_git_dir dt_repo_name dt_state_dir dt_head dt_harness dt_session_id; do compare_paths "$function"; done
  compare_paths dt_path '~/space here'
  compare_paths dt_path 'literal/~'
  git -c user.name=Tester -c user.email=tester@example.com commit --quiet --allow-empty -m fixture
  git worktree add --quiet -b linked "$BATS_TEST_TMPDIR/linked"
  cd "$BATS_TEST_TMPDIR/linked"
  for function in dt_root dt_git_dir dt_repo_name dt_state_dir dt_head; do compare_paths "$function"; done
}

@test "AC6: harness values and every session variable precedence combination match" {
  export _DEVTOOLS_AGENTREPO_BIN="$(command -v agentrepo)"
  for ((bits=0; bits<16; bits++)); do
    unset DEVTOOLS_HARNESS CLAUDE_CODE_SESSION_ID CLAUDECODE CODEX_THREAD_ID
    ((bits & 1)) && export DEVTOOLS_HARNESS=custom
    ((bits & 2)) && export CLAUDE_CODE_SESSION_ID=claude-session
    ((bits & 4)) && export CLAUDECODE=1
    ((bits & 8)) && export CODEX_THREAD_ID=codex-session
    compare_paths dt_harness
    compare_paths dt_session_id
  done
  for DEVTOOLS_HARNESS in claude codex unknown; do
    export DEVTOOLS_HARNESS
    compare_paths dt_harness
  done
}

@test "AC10: checksum producer failures preserve both streams even after printing" {
  export _DEVTOOLS_AGENTREPO_BIN="$(command -v agentrepo)"
  mkdir bin
  export PATH="$PWD/bin:$PATH"
  for text in '' 'abc  draft'; do
    printf '#!/bin/sh\nprintf "%%s" "%s"\nexit 9\n' "$text" >bin/sha256sum
    chmod +x bin/sha256sum
    compare_paths dt_digest draft
  done
}

@test "AC5: supported Git failures preserve the diagnostic and discard partial values" {
  export _DEVTOOLS_AGENTREPO_BIN="$(command -v agentrepo)"
  export _DEVTOOLS_AGENTREPO_CONTEXT=true
  mkdir bin
  export PATH="$PWD/bin:$PATH"
  for message in 'not a repository' 'rev-parse failed' 'unreadable Git directory'; do
    printf '#!/bin/sh\necho partial\necho "%s" >&2\nexit 9\n' "$message" >bin/git
    chmod +x bin/git
    for function in dt_root dt_git_dir dt_repo_name dt_state_dir dt_head dt_agent_context; do
      run --separate-stderr bash -c '. "$LIB"; "$@"' _ "$function"
      [ "$status" -eq 9 ]
      [ -z "$output" ]
      [ "$stderr" = "$message" ]
    done
    run --separate-stderr bash "$ROOT/packages/agents/source/lib/worktree-ready.sh"
    [ "$status" -eq 9 ]
    [ -z "$output" ]
    [ "$stderr" = "$message" ]
  done
}

@test "AC7: supported readiness preserves the shell transcript and never changes trust" {
  export _DEVTOOLS_AGENTREPO_BIN="$(command -v agentrepo)"
  export READY="$ROOT/packages/agents/source/lib/worktree-ready.sh"
  mkdir bin
  export PATH="$PWD/bin:$PATH"
  printf '' >mise.toml
  cat >bin/mise <<'STUB'
#!/bin/sh
case "$*" in
*'trust --show') printf 'fixture: untrusted\n' ;;
*) touch trust-changed; exit 99 ;;
esac
STUB
  chmod +x bin/mise
  export _DEVTOOLS_AGENTREPO_CONTEXT=false
  run --separate-stderr bash "$READY"
  local want_status=$status want_out=$output want_err=$stderr
  export _DEVTOOLS_AGENTREPO_CONTEXT=true
  run --separate-stderr bash "$READY"
  [ "$status" -eq "$want_status" ]
  [ "$output" = "$want_out" ]
  [ "$stderr" = "$want_err" ]
  [ ! -e trust-changed ]
}

@test "AC2: successful discovery keeps producer diagnostics on stderr" {
  export _DEVTOOLS_AGENTREPO_BIN="$(command -v agentrepo)"
  mkdir bin
  export PATH="$PWD/bin:$PATH"
  printf '#!/bin/sh\necho /fixture\necho warning >&2\n' >bin/git
  chmod +x bin/git
  compare_paths dt_root
}

@test "AC7 AC9: standalone readiness ships its discovery bridge without the full library" {
  export _DEVTOOLS_AGENTREPO_BIN="$(command -v agentrepo)"
  export _DEVTOOLS_AGENTREPO_CONTEXT=true
  mkdir standalone
  cp "$ROOT/packages/agents/source/lib/worktree-ready.sh" "$ROOT/packages/agents/source/lib/context.sh" standalone/
  run --separate-stderr bash standalone/worktree-ready.sh
  [ "$status" -eq 0 ]
  [ -z "$output" ]
  [ -z "$stderr" ]
  [ ! -e standalone/lib.sh ]
}

@test "AC6 AC8: the bridge preserves literal shell paths for a native command" {
  export HOME='/c/shell home' EXPECTED_SHELL_HOME='/c/shell home'
  export _DEVTOOLS_AGENTREPO_CONTEXT=true MSYS2_ARG_CONV_EXCL=original
  cat >"$_DEVTOOLS_AGENTREPO_BIN" <<'STUB'
#!/bin/sh
[ "$MSYS2_ARG_CONV_EXCL" = '*' ] || exit 90
while [ "$#" -gt 0 ]; do
  if [ "$1" = --home ]; then
    shift
    [ "$1" = "$EXPECTED_SHELL_HOME" ] || exit 91
    printf 'devtools-context-v1\n/c/shell home/docs\ndevtools-context-end\n'
    exit 0
  fi
  shift
done
exit 92
STUB
  run --separate-stderr bash -c '. "$LIB"; dt_path "~/docs"'
  [ "$status" -eq 0 ]
  [ "$output" = '/c/shell home/docs' ]
  [ -z "$stderr" ]
  [ "$MSYS2_ARG_CONV_EXCL" = original ]
}
