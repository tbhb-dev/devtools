#!/usr/bin/env bats

setup() {
  SCRIPT="${BATS_TEST_DIRNAME}/../../../../.claude/skills/release/scripts/watch-release.sh"
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
  export RELEASE_RUN_APPEAR_TIMEOUT=0 RELEASE_RUN_TIMEOUT=0
  mkdir -p "${BATS_TEST_TMPDIR}/bin"
  cat >"${BATS_TEST_TMPDIR}/bin/gh" <<'STUB'
#!/usr/bin/env bash
case "$1 $2" in
"auth status")
  [ "${3:-}" = "--active" ] && [ "${AUTH_FAIL:-0}" = 0 ]
  ;;
"run list") printf '42\thttps://github.com/o/r/actions/runs/42\n' ;;
"run view")
  case "$5" in
  jobs) printf 'success\tRelease\n' ;;
  status,conclusion) printf 'completed\tsuccess\n' ;;
  *) exit 1 ;;
  esac
  ;;
*) exit 1 ;;
esac
STUB
  chmod +x "${BATS_TEST_TMPDIR}/bin/gh"
  export PATH="${BATS_TEST_TMPDIR}/bin:$PATH"
  git init --quiet "${BATS_TEST_TMPDIR}/repo"
  cd "${BATS_TEST_TMPDIR}/repo"
}

@test "watches the release when only the active account is authenticated" {
  run gh auth status
  [ "$status" -eq 1 ]

  run bash "$SCRIPT" none
  [ "$status" -eq 0 ]
  [[ "$output" == *"PASS  Release"* ]]
  [[ "$output" == *"RELEASE RUN SUCCEEDED"* ]]
}

@test "refuses to watch when the active account is not authenticated" {
  export AUTH_FAIL=1

  run bash "$SCRIPT" none
  [ "$status" -eq 2 ]
  [[ "$output" == *"ERROR gh is not authenticated"* ]]
  [[ "$output" != *"RELEASE RUN 42"* ]]
}
