#!/usr/bin/env bats
# The watch-pr skill's wrapper entry point. Every case runs with a PATH that
# holds only bash and the case's own agentrepo stub, so an installed agentrepo
# cannot change which branch the wrapper takes.

setup() {
  bats_require_minimum_version 1.5.0
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
  SKILL="${ROOT}/packages/agents/claude/.apm/skills/watch-pr/scripts"
  CHECKS="${BATS_TEST_TMPDIR}/checks"
  BIN="${BATS_TEST_TMPDIR}/bin"
  SCRIPTS="${BATS_TEST_TMPDIR}/scripts"
  mkdir -p "$CHECKS" "$BIN" "$SCRIPTS"
  ln -s "$(command -v bash)" "$BIN/bash"
  cp "$SKILL/watch.sh" "$SCRIPTS/watch.sh"
  cat >"$SCRIPTS/watch-checks.sh" <<'FALLBACK'
#!/usr/bin/env bash
printf 'fallback\n'
printf '<%s>\n' "$@"
exit 19
FALLBACK
  export CHECKS
}

wrapper() {
  run --separate-stderr env PATH="$BIN" bash "$SCRIPTS/watch.sh" "$@"
}

supporting_binary() {
  cat >"$BIN/agentrepo" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$CHECKS/probes"
if [ "$*" = 'checks watch --help' ]; then
  printf 'Usage: agentrepo checks watch [pull-request-number]\n'
  exit 0
fi
printf 'stdout\000bytes\n'
printf 'stderr\n' >&2
printf '<%s>\n' "$@"
exit "$WATCH_STATUS"
STUB
  chmod +x "$BIN/agentrepo"
}

@test "a supporting binary receives the arguments and its bytes and statuses pass through" {
  supporting_binary
  for WATCH_STATUS in 0 1 2 76; do
    export WATCH_STATUS
    wrapper '7 8' '' '*'
    [ "$status" -eq "$WATCH_STATUS" ]
    # Bats strips NUL bytes; the original streams are compared as files below.
    [[ "$output" == *$'<checks>\n<watch>\n<7 8>\n<>\n<*>' ]]
    [ "$stderr" = stderr ]
    [[ "$output" != *fallback* ]]
  done
  run cat "$CHECKS/probes"
  [ "${#lines[@]}" -eq 8 ]
  [ "${lines[0]}" = 'checks watch --help' ]
  printf 'stdout\000bytes\n<checks>\n<watch>\n<7 8>\n<>\n<*>\n' >"$CHECKS/expected"
  WATCH_STATUS=0 PATH="$BIN" bash "$SCRIPTS/watch.sh" '7 8' '' '*' >"$CHECKS/out" 2>"$CHECKS/err"
  cmp "$CHECKS/expected" "$CHECKS/out"
  printf 'stderr\n' >"$CHECKS/expected"
  cmp "$CHECKS/expected" "$CHECKS/err"
}

@test "an absent binary falls back with the arguments and status unchanged" {
  wrapper '7 8' '' '*'
  [ "$status" -eq 19 ]
  [ "$output" = $'fallback\n<7 8>\n<>\n<*>' ]
  [ -z "$stderr" ]
}

@test "an older binary with generic help or a rejected subcommand falls back" {
  cat >"$BIN/agentrepo" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$CHECKS/probes"
printf 'Usage: agentrepo [command]\n'
printf 'unsupported\n' >&2
exit "$PROBE_STATUS"
STUB
  chmod +x "$BIN/agentrepo"
  for PROBE_STATUS in 0 1; do
    export PROBE_STATUS
    wrapper 7
    [ "$status" -eq 19 ]
    [ "$output" = $'fallback\n<7>' ]
    [ -z "$stderr" ]
  done
  run cat "$CHECKS/probes"
  [ "${#lines[@]}" -eq 2 ]
  [ "${lines[0]}" = 'checks watch --help' ]
  [ "${lines[1]}" = 'checks watch --help' ]
}

@test "the wrapper and the frozen watcher match the source in both packages" {
  local source=packages/agents/source/skills/watch-pr/shared/scripts
  for harness in claude codex; do
    cmp "$ROOT/$source/watch-checks.sh" "$ROOT/packages/agents/$harness/.apm/skills/watch-pr/scripts/watch-checks.sh"
    cmp "$ROOT/$source/watch.sh" "$ROOT/packages/agents/$harness/.apm/skills/watch-pr/scripts/watch.sh"
  done
}
