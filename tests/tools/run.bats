#!/usr/bin/env bats

setup() {
  bats_require_minimum_version 1.5.0
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  SOURCE=$(cd "$BATS_TEST_DIRNAME/../.." && pwd)
  cd "$BATS_TEST_TMPDIR"
  mkdir -p cmd/child tools payload/scripts
  awk '
    /^\[/ { copy = ($0 == "[tasks.run]") }
    copy { print }
  ' "$SOURCE/mise.toml" >mise.toml
  awk '
    /^\[/ { copy = ($0 == "[\"devtools:build\"]") }
    copy && /^\[/ { print "[tasks.\"devtools:build\"]"; next }
    copy { print }
  ' "$SOURCE/.devtools/tasks/go.toml" >>mise.toml
  cat >>mise.toml <<'TOML'
[env]
DEVTOOLS_DIR = "{{ config_root }}/payload"
DEVTOOLS_GO_MODULE = "example.com/launcher"
TOML
  awk '$1 == "go" && $2 == "=" { print "[tools]"; print }' \
    "$SOURCE/.config/mise/conf.d/10-devtools-tools.toml" >>mise.toml
  cp "$SOURCE/.config/mise/mise.lock" mise.lock
  cp "$SOURCE/tools/go-ldflags.sh" tools/go-ldflags.sh
  cp "$SOURCE/.devtools/scripts/go-ldflags.sh" payload/scripts/go-ldflags.sh
  export MISE_TRUSTED_CONFIG_PATHS="$PWD"
  # CI forces color; the expected stderr has no escape codes.
  export NO_COLOR=1 MISE_COLOR=0
  export CHILD_COUNTER="$PWD/counter"
  : >"$CHILD_COUNTER"
  printf 'module example.com/launcher\n' >go.mod
  cat >cmd/child/main.go <<'GO'
package main

import (
    "fmt"
    "io"
    "os"
    "strconv"
)

func main() {
    counter, err := os.OpenFile(os.Getenv("CHILD_COUNTER"), os.O_APPEND|os.O_WRONLY, 0600)
    if err != nil {
        panic(err)
    }
    _, _ = counter.WriteString("x")
    _ = counter.Close()
    fmt.Printf("original %q\n", os.Args[3:])
    _, _ = io.Copy(os.Stdout, os.Stdin)
    if os.Args[2] == "bytes" {
        _, _ = os.Stderr.Write([]byte("child\x00stderr\nexit status 3\nno newline"))
    }
    status, _ := strconv.Atoi(os.Args[1])
    os.Exit(status)
}
GO
  printf 'stdin\000bytes\n' >stdin
}

# Files preserve NUL bytes and trailing newlines that Bats' output strips.
launch() {
  mise --quiet run run child "$@" <stdin >stdout 2>stderr
}

@test "run forwards every exit status once and preserves arguments and both streams" {
  local code mode count=0
  printf 'original ["two words" "" "--flag=*?" "child"]\n' >expected.stdout
  cat stdin >>expected.stdout
  for mode in empty bytes; do
    for code in {3..255} {0..2}; do
      run "-$code" launch "$code" "$mode" 'two words' '' '--flag=*?' child
      [ "$status" -eq "$code" ]
      count=$((count + 1))
      [ "$(wc -c <"$CHILD_COUNTER")" -eq "$count" ]
      cmp expected.stdout stdout
      : >expected.stderr
      if [[ $mode == bytes ]]; then
        printf 'child\000stderr\nexit status 3\nno newline' >expected.stderr
      fi
      if [[ $code != 0 ]]; then
        printf '[run] ERROR task failed\n' >>expected.stderr
      fi
      cmp expected.stderr stderr
    done
  done
}

@test "run builds from a cold checkout without a manual build" {
  rm -rf bin
  run launch 0 empty
  [ "$status" -eq 0 ]
  [ -x bin/child ]
  [ "$(cat "$CHILD_COUNTER")" = x ]
}

@test "run rebuilds source edits on the next invocation" {
  run launch 0 empty
  [ "$status" -eq 0 ]
  sed 's/original/edited/' cmd/child/main.go >edited.go
  mv edited.go cmd/child/main.go
  run launch 0 empty
  [ "$status" -eq 0 ]
  printf 'edited []\n' >expected.stdout
  cat stdin >>expected.stdout
  cmp expected.stdout stdout
  [ "$(cat "$CHILD_COUNTER")" = xx ]
}

@test "run never launches an older binary after a failed build" {
  run launch 0 empty
  [ "$status" -eq 0 ]
  [ -x bin/child ]
  printf '\ninvalid Go source\n' >>cmd/child/main.go
  run launch 0 empty
  [ "$status" -ne 0 ]
  [ -x bin/child ]
  [ "$(cat "$CHILD_COUNTER")" = x ]
  [ ! -s stdout ]
}

@test "run preserves metadata failure status 23 and never starts the child" {
  run launch 0 empty
  [ "$status" -eq 0 ]
  printf 'echo flags; exit 23\n' >payload/scripts/go-ldflags.sh
  run launch 0 empty
  [ "$status" -eq 23 ]
  [ "$(cat "$CHILD_COUNTER")" = x ]
  [ ! -s stdout ]
}
