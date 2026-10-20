#!/usr/bin/env bats

@test "bootstrap installs working-tree commands before git hooks" {
  root=$(git -C "$BATS_TEST_DIRNAME" rev-parse --show-toplevel)

  run mise -C "$root" task info bootstrap --json

  [ "$status" -eq 0 ]
  commands=$(jq -r '.run[]' <<<"$output")
  [ "$commands" = $'mise run install-toolchain\nmise run install-tools\nmise run install\nmise run devtools:prek-install\nmise run devtools:install-pre-push-hook' ]
}
