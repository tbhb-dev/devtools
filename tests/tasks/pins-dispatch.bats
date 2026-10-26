#!/usr/bin/env bats

setup() {
  bats_require_minimum_version 1.5.0
  ROOT="${BATS_TEST_DIRNAME}/../.."
  WORK="$BATS_TEST_TMPDIR/consumer"
  BIN="$BATS_TEST_TMPDIR/bin"
  mkdir -p "$WORK/.devtools/scripts" "$BIN"
  awk '
    $0 == "[\"devtools:check-pins\"]" { selected = 1; next }
    selected && /^run = / { body = 1; next }
    body && /^\047\047\047$/ { exit }
    body { print }
  ' "$ROOT/.devtools/tasks/pins.toml" > "$WORK/check.sh"
  cat > "$WORK/.devtools/scripts/check-pins-legacy.sh" <<'SH'
#!/usr/bin/env bash
printf 'fallback %s\n' "$*"
printf 'legacy stderr\n' >&2
exit 19
SH
  cat > "$BIN/mise" <<'SH'
#!/usr/bin/env bash
[ "$*" = 'which agentrepo' ] || exit 99
[ -n "${SELECTED:-}" ] || exit 1
printf '%s\n' "$SELECTED"
SH
  cat > "$BIN/agentrepo" <<'SH'
#!/usr/bin/env bash
if [ "$*" = 'check-pins --help' ]; then
  printf 'Usage:\n  agentrepo check-pins [flags]\n'
  exit 0
fi
printf 'go %s %s\n' "$*" "$CONSUMER_ENV"
printf 'go stderr\n' >&2
exit "${GO_STATUS:-0}"
SH
  chmod +x "$BIN/"*
  export PATH="$BIN:$PATH" CONSUMER_ENV=preserved
  export SELECTED="$BIN/agentrepo"
  cd "$WORK"
}

@test "capable checker preserves arguments environment and streams" {
  run --separate-stderr bash check.sh 'space argument' --value
  [ "$status" -eq 0 ]
  [ "$output" = 'go check-pins space argument --value preserved' ]
  [ "$stderr" = 'go stderr' ]
}

@test "operational failure never retries through shell" {
  run --separate-stderr env GO_STATUS=23 bash check.sh
  [ "$status" -eq 23 ]
  [ "$output" = 'go check-pins preserved' ]
  [ "$stderr" = 'go stderr' ]
}

@test "missing selected binary falls back without using ambient binary" {
  run --separate-stderr env SELECTED= bash check.sh 'space argument'
  [ "$status" -eq 19 ]
  [ "$output" = 'fallback space argument' ]
  [ "$stderr" = 'legacy stderr' ]
}

@test "selected older binary beside a capable ambient binary falls back" {
  printf '#!/usr/bin/env bash\nprintf "unknown command\\n" >&2\nexit 5\n' > "$BIN/older"
  chmod +x "$BIN/older"
  run --separate-stderr env SELECTED="$BIN/older" bash check.sh
  [ "$status" -eq 19 ]
  [ "$output" = 'fallback ' ]
  [ "$stderr" = 'legacy stderr' ]
}

@test "unsupported executable and failed help producer fall back" {
  printf '#!/usr/bin/env bash\necho unsupported\n' > "$BIN/unsupported"
  chmod +x "$BIN/unsupported"
  run bash -c 'SELECTED="$1" bash check.sh' bash "$BIN/unsupported"
  [ "$status" -eq 19 ]
  printf '#!/usr/bin/env bash\necho "agentrepo check-pins"\nexit 23\n' > "$BIN/unsupported"
  run bash -c 'SELECTED="$1" bash check.sh' bash "$BIN/unsupported"
  [ "$status" -eq 19 ]
}

@test "non executable selected path falls back" {
  chmod -x "$BIN/agentrepo"
  run bash check.sh
  [ "$status" -eq 19 ]
}
