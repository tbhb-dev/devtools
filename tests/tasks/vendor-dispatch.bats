#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
  WORK="$BATS_TEST_TMPDIR/consumer"
  mkdir -p "$WORK/bin" "$WORK/pinned" "$WORK/vendor" "$WORK/.devtools"
  cd "$WORK"
  git init -q
  printf 'tracked\n' >vendor/modules.txt
  git add -- vendor/modules.txt
  export DEVTOOLS_DIR="$ROOT/.devtools"
  export RECORD="$WORK/record"
  export PATH="$WORK/bin:$PATH"
  cat >bin/mise <<'SH'
#!/usr/bin/env bash
[ "${SELECT_PIN:-no}" = yes ] || exit 1
printf '%s/pinned/agentrepo\n' "$PWD"
SH
  cat >bin/agentrepo <<'SH'
#!/usr/bin/env bash
if [ "${2:-}" = --help ]; then
  printf 'probe %s\n' "$1" >>"$RECORD"
  [ "${CAPABLE:-yes}" = yes ]
  exit $?
fi
printf 'command' >>"$RECORD"
printf ' <%s>' "$@" >>"$RECORD"
printf '\n' >>"$RECORD"
exit "${RESULT:-0}"
SH
  cat >bin/go <<'SH'
#!/usr/bin/env bash
printf 'go %s\n' "$*" >>"$RECORD"
SH
  cat >bin/vendir <<'SH'
#!/usr/bin/env bash
printf 'vendir %s\n' "$*" >>"$RECORD"
SH
  cp bin/agentrepo pinned/agentrepo
  chmod +x bin/* pinned/*
}

@test "capable vendoring dispatch forwards arguments exactly" {
  for command in vendor-check check-vendored; do
    run bash "$DEVTOOLS_DIR/scripts/vendoring-check.sh" "$command" 'with space' '--literal'
    [ "$status" -eq 0 ]
    [[ "$(cat "$RECORD")" == *"command <$command> <with space> <--literal>"* ]]
  done
  [[ "$(cat "$RECORD")" != *'go mod'* ]]
  [[ "$(cat "$RECORD")" != *'vendir sync'* ]]
}

@test "old and unsupported binaries use frozen vendoring fallback" {
  export CAPABLE=no
  for command in vendor-check check-vendored; do
    run bash "$DEVTOOLS_DIR/scripts/vendoring-check.sh" "$command"
    [ "$status" -eq 0 ]
  done
  [[ "$(cat "$RECORD")" == *'go mod tidy'* ]]
  [[ "$(cat "$RECORD")" == *'go mod vendor'* ]]
  [[ "$(cat "$RECORD")" == *'vendir sync'* ]]
}

@test "absent binary uses frozen vendoring fallback" {
  rm bin/agentrepo
  ln -s "$(command -v bash)" bin/bash
  ln -s "$(command -v git)" bin/git
  local original_path=$PATH
  export PATH="$WORK/bin"
  run bash "$DEVTOOLS_DIR/scripts/vendoring-check.sh" vendor-check
  export PATH=$original_path
  [ "$status" -eq 0 ]
  [[ "$(cat "$RECORD")" == *'go mod tidy'* ]]
}

@test "consumer-selected binary takes precedence over PATH binary" {
  export SELECT_PIN=yes
  export CAPABLE=no
  sed 's/${CAPABLE:-yes}/yes/' bin/agentrepo >pinned/agentrepo
  run bash "$DEVTOOLS_DIR/scripts/vendoring-check.sh" vendor-check
  [ "$status" -eq 0 ]
  [[ "$(cat "$RECORD")" == *'command <vendor-check>'* ]]
  [[ "$(cat "$RECORD")" != *'go mod'* ]]
}

@test "operational failure never reruns regeneration through shell" {
  export RESULT=23
  for command in vendor-check check-vendored; do
    run bash "$DEVTOOLS_DIR/scripts/vendoring-check.sh" "$command"
    [ "$status" -eq 23 ]
  done
  [[ "$(cat "$RECORD")" != *'go mod'* ]]
  [[ "$(cat "$RECORD")" != *'vendir sync'* ]]
}
