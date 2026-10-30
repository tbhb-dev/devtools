#!/usr/bin/env bats
# cspell:words CDPATH
# Capability detection must never turn a supported failure into a second diagnosis.

setup() {
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
  SCRIPT="${ROOT}/packages/agents/codex/.apm/skills/fix-pr/scripts/diagnose.sh"
  export CALLS="${BATS_TEST_TMPDIR}/calls"
  export RESULT=0 SUPPORTED=1
  mkdir -p "${BATS_TEST_TMPDIR}/bin" "${BATS_TEST_TMPDIR}/entry"
  cp "$SCRIPT" "${BATS_TEST_TMPDIR}/entry/diagnose.sh"
  # The sentinel proves argument and status forwarding without invoking GitHub.
  cat >"${BATS_TEST_TMPDIR}/entry/diagnose-legacy.sh" <<'SH'
#!/usr/bin/env bash
printf 'legacy\n' >>"$CALLS"
printf '<%s>\n' "$@"
exit 17
SH
  cat >"${BATS_TEST_TMPDIR}/bin/agentrepo" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$CALLS"
if [ "$*" = 'checks diagnose --help' ]; then
  if [ "$SUPPORTED" = generic ]; then
    printf 'Usage: agentrepo checks [command]\n'
    exit 0
  fi
  [ "$SUPPORTED" = 1 ] || exit 1
  printf 'Usage: agentrepo checks diagnose [pull-request-number]\n'
  exit 0
fi
shift 2
printf '<%s>\n' "$@"
printf 'diagnostic stderr\n' >&2
exit "$RESULT"
SH
  chmod +x "${BATS_TEST_TMPDIR}/bin/agentrepo"
  export PATH="${BATS_TEST_TMPDIR}/bin:$PATH"
}

@test "AC12 supported statuses and arguments pass through without fallback" {
  for RESULT in 0 1 2; do
    export RESULT
    : >"$CALLS"
    run bash "$SCRIPT" '794' 'argument with spaces' ''
    [ "$status" -eq "$RESULT" ]
    [[ "$output" == $'<794>\n<argument with spaces>\n<>\ndiagnostic stderr' ]]
    [ "$(wc -l <"$CALLS")" -eq 2 ]
    ! grep -q legacy "$CALLS"
  done
}

@test "AC12 an unsupported subcommand probes once and uses frozen fallback" {
  export SUPPORTED=0
  run bash "${BATS_TEST_TMPDIR}/entry/diagnose.sh" '794' 'argument with spaces' ''
  [ "$status" -eq 17 ]
  [[ "$output" == $'<794>\n<argument with spaces>\n<>' ]]
  [ "$(cat "$CALLS")" = $'checks diagnose --help\nlegacy' ]
}

@test "AC12 an absent binary uses frozen fallback" {
  rm "${BATS_TEST_TMPDIR}/bin/agentrepo"
  # A PATH with only bash and the wrapper's directory tools excludes host binaries.
  for tool in bash dirname; do
    ln -s "$(command -v "$tool")" "${BATS_TEST_TMPDIR}/bin/$tool"
  done
  run env PATH="${BATS_TEST_TMPDIR}/bin" bash "${BATS_TEST_TMPDIR}/entry/diagnose.sh" 794
  [ "$status" -eq 17 ]
  [ "$output" = '<794>' ]
  [ "$(cat "$CALLS")" = legacy ]
}

@test "AC12 frozen fallback remains byte identical to the pinned source" {
  expected=276ecddd4d8cdefc39c5d2370a5269f7b283dc05
  for script in \
    packages/agents/source/skills/fix-pr/shared/scripts/diagnose-legacy.sh \
    packages/agents/claude/.apm/skills/fix-pr/scripts/diagnose-legacy.sh \
    packages/agents/codex/.apm/skills/fix-pr/scripts/diagnose-legacy.sh \
    .agents/skills/fix-pr/scripts/diagnose-legacy.sh \
    .claude/skills/fix-pr/scripts/diagnose-legacy.sh; do
    [ "$(git hash-object "${ROOT}/$script")" = "$expected" ]
  done
}

@test "AC12 a legacy group returning generic help with success still falls back" {
  export SUPPORTED=generic
  run bash "${BATS_TEST_TMPDIR}/entry/diagnose.sh" 794
  [ "$status" -eq 17 ]
  [ "$output" = '<794>' ]
  [ "$(cat "$CALLS")" = $'checks diagnose --help\nlegacy' ]
}

@test "AC12 fallback ignores hostile CDPATH for relative invocation" {
  export SUPPORTED=0
  mkdir -p "${BATS_TEST_TMPDIR}/hostile/entry"
  cd "$BATS_TEST_TMPDIR"
  run env CDPATH="${BATS_TEST_TMPDIR}/hostile:${BATS_TEST_TMPDIR}" bash entry/diagnose.sh 794
  [ "$status" -eq 17 ]
  [ "$output" = '<794>' ]
  [ "$(cat "$CALLS")" = $'checks diagnose --help\nlegacy' ]
}
