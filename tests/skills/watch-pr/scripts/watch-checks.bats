#!/usr/bin/env bats
# The watch-pr skill's watcher, as generated. gh is a stub on PATH that
# answers each `gh pr checks` call with the next numbered response in a
# scratch directory and counts the calls, so a case scripts what the
# checks look like poll by poll.

setup() {
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
  SCRIPT="${ROOT}/packages/agents/claude/.apm/skills/watch-pr/scripts/watch-checks.sh"
  REPO="${BATS_TEST_TMPDIR}/repo"
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
  export PR_CHECKS_INTERVAL=0 PR_CHECKS_APPEAR_TIMEOUT=0 PR_CHECKS_TIMEOUT=60
  export CHECKS="${BATS_TEST_TMPDIR}/checks"
  mkdir -p "$CHECKS" "${BATS_TEST_TMPDIR}/bin"
  cat >"${BATS_TEST_TMPDIR}/bin/gh" <<'EOF'
#!/usr/bin/env bash
case "$1 $2" in
"auth status") exit 0 ;;
"api graphql") echo agent ;;
"pr checks")
  n=$(($(cat "$CHECKS/count" 2>/dev/null || echo 0) + 1))
  printf '%s\n' "$n" >"$CHECKS/count"
  [ -f "$CHECKS/$n" ] || n=$(ls "$CHECKS" | grep -E '^[0-9]+$' | sort -n | tail -n 1)
  if [ -f "$CHECKS/$n.err" ]; then
    cat "$CHECKS/$n.err" >&2
    exit 1
  fi
  cat "$CHECKS/$n"
  ;;
esac
EOF
  chmod +x "${BATS_TEST_TMPDIR}/bin/gh"
  export PATH="${BATS_TEST_TMPDIR}/bin:$PATH"
  git init --quiet "$REPO"
  cd "$REPO"
}

check() { printf '{"name":"%s","bucket":"%s","link":"https://github.com/o/r/actions/runs/1/job/%s","workflow":"%s"}' "$1" "$2" "$3" "$4"; }
calls() { cat "$CHECKS/count"; }

@test "each poll reads the checks through one gh call" {
  printf '[%s]\n' "$(check build pending 5 ci)" >"$CHECKS/1"
  printf '[%s]\n' "$(check build pending 5 ci)" >"$CHECKS/2"
  printf '[%s]\n' "$(check build pending 5 ci)" >"$CHECKS/3"
  printf '[%s]\n' "$(check build pass 5 ci)" >"$CHECKS/4"

  run bash "$SCRIPT" 7

  [ "$status" -eq 0 ]
  [[ "$output" == *"PASS  build"* ]]
  [[ "$output" == *"ALL GREEN #7"* ]]
  [ "$(calls)" -eq 4 ]
}

@test "a response that is not a list of checks fails loudly" {
  printf '{"message":"Bad credentials"}\n' >"$CHECKS/1"

  run bash "$SCRIPT" 7

  [ "$status" -eq 2 ]
  [[ "$output" == *"ERROR gh returned an invalid checks response for #7"* ]]
}

@test "a gh failure stops the watch rather than reading as nothing pending" {
  : >"$CHECKS/1"
  printf 'HTTP 502: Bad Gateway\n' >"$CHECKS/1.err"

  run bash "$SCRIPT" 7

  [ "$status" -eq 2 ]
  [[ "$output" == *"ERROR could not read checks for #7: HTTP 502"* ]]
}

@test "a pull request with no checks is inconclusive, not green" {
  : >"$CHECKS/1"
  printf "no checks reported on the 'topic' branch\n" >"$CHECKS/1.err"

  run bash "$SCRIPT" 7

  [ "$status" -eq 2 ]
  [[ "$output" == *"NO CHECKS #7"* ]]
  [[ "$output" != *"ALL GREEN"* ]]
}

@test "a rerun replaces the failure it repeats" {
  printf '[%s,%s]\n' "$(check build fail 5 ci)" "$(check build pass 9 ci)" >"$CHECKS/1"

  run bash "$SCRIPT" 7

  [ "$status" -eq 0 ]
  [[ "$output" != *"FAIL"* ]]
  [[ "$output" == *"ALL GREEN #7"* ]]
}

@test "two workflows sharing a job name are reported apart" {
  printf '[%s,%s]\n' "$(check build pass 5 lint)" "$(check build fail 6 test)" >"$CHECKS/1"

  run bash "$SCRIPT" 7

  [ "$status" -eq 1 ]
  [[ "$output" == *"PASS  build"* ]]
  [[ "$output" == *"FAIL  build  https://github.com/o/r/actions/runs/1/job/6"* ]]
  [[ "$output" == *"FAILED #7 with 1 failing check(s)"* ]]
}

@test "quota exhaustion stops the watcher with a distinct status" {
  : >"$CHECKS/1"
  printf 'gh: API rate limit exceeded for user ID 1. (HTTP 403)\n' >"$CHECKS/1.err"
  run bash "$SCRIPT" 7
  [ "$status" -eq 76 ]
  [[ "$output" == *"primary GraphQL rate limit"* ]]
  [[ "$output" == *"the reset time isn't known from the failure"* ]]
  [ "$(calls)" -eq 1 ]
}

@test "authentication errors stop the watcher without quota advice" {
  : >"$CHECKS/1"
  printf 'HTTP 401: Bad credentials\n' >"$CHECKS/1.err"
  run bash "$SCRIPT" 7
  [ "$status" -eq 2 ]
  [[ "$output" == *"Bad credentials"* ]]
  [[ "$output" != *"quota exhausted"* ]]
  [ "$(calls)" -eq 1 ]
}

@test "a check count printed before jq fails cannot report green" {
  printf '[%s]\n' "$(check build pass 5 ci)" >"$CHECKS/1"
  export REAL_JQ
  REAL_JQ=$(command -v jq)
  cat >"${BATS_TEST_TMPDIR}/bin/jq" <<'STUB'
#!/usr/bin/env bash
"$REAL_JQ" "$@" || exit $?
[ "$*" != length ] || exit 9
STUB
  chmod +x "${BATS_TEST_TMPDIR}/bin/jq"
  run bash "$SCRIPT" 7
  [ "$status" -eq 2 ]
  [[ "$output" == *"could not count checks"* ]]
  [[ "$output" != *"ALL GREEN"* ]]
}
