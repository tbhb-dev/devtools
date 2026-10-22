# Keep scratch repositories independent of the invoking Git environment.
isolate_git() {
  local name
  for name in ${!GIT_CONFIG_@} ${!GIT_AUTHOR_@} ${!GIT_COMMITTER_@}; do
    unset "$name"
  done
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
}

# Gate runners are deterministic; no consumer toolchain runs in fixtures.
stub_gates() {
  mkdir -p "$BATS_TEST_TMPDIR/bin"
  export GATE_ARGS_LOG="$BATS_TEST_TMPDIR/gate-args"
  export GATE_LOG="$BATS_TEST_TMPDIR/gates.log"
  : >"$GATE_LOG"
  cat >"$BATS_TEST_TMPDIR/bin/mise" <<'STUB'
#!/usr/bin/env bash
case "$1" in
--version) printf '%s\n' "${GATE_VERSION:-mise-test}" ;;
ls) printf '%s\n' "${TOOL_VERSION:-{}}" ;;
run)
  printf '%s\n' "$2" >>"$GATE_LOG"
  printf '%s\n' "$*" >>"$GATE_ARGS_LOG"
  [ "$2" != "${FAIL_GATE:-}" ] || exit 1
  ;;
esac
STUB
  chmod +x "$BATS_TEST_TMPDIR/bin/mise"
  export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
}
