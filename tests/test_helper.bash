# Keep scratch repositories independent of the invoking Git environment.
isolate_git() {
  # Scratch workflows select their mode through their own entry points.
  unset DEVTOOLS_UNATTENDED DT_UNATTENDED
  local name
  for name in ${!GIT_CONFIG_@} ${!GIT_AUTHOR_@} ${!GIT_COMMITTER_@}; do
    unset "$name"
  done
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
}

# Gate runners are deterministic; no consumer toolchain runs in fixtures.
stub_gates() {
  mkdir -p "$BATS_TEST_TMPDIR/bin"
  export GATE_LOG="$BATS_TEST_TMPDIR/gates.log"
  : >"$GATE_LOG"
  export AFFECTED_LOG="$BATS_TEST_TMPDIR/affected.log"
  : >"$AFFECTED_LOG"
  cat >"$BATS_TEST_TMPDIR/bin/mise" <<'STUB'
#!/usr/bin/env bash
case "$1" in
--version) printf '%s\n' "${GATE_VERSION:-mise-test}" ;;
tasks) printf '%s\n' "${AFFECTED_TASKS:-test-affected}" ;;
ls) printf '%s\n' "${TOOL_VERSION:-{}}" ;;
run)
  printf '%s\n' "$2" >>"$GATE_LOG"
  if [ "$2" = test-affected ]; then
    printf '%s\n' "$*" >>"$AFFECTED_LOG"
    printf 'No affected tests were selected.\n'
  fi
  [ "$2" != "${FAIL_GATE:-}" ] || exit 1
  [ "$2" != test-affected ] || [ "${MISSING_TEST_BINARY:-0}" != 1 ] || exec missing-test-binary
  ;;
esac
STUB
  chmod +x "$BATS_TEST_TMPDIR/bin/mise"
  export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
}
