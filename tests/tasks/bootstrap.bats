#!/usr/bin/env bats

@test "bootstrap builds working-tree commands before git hooks" {
  root=$(git -C "$BATS_TEST_DIRNAME" rev-parse --show-toplevel)

  run mise -C "$root" task info bootstrap --json

  [ "$status" -eq 0 ]
  commands=$(jq -r '.run[]' <<<"$output")
  [ "$commands" = $'mise run install-toolchain\nmise run install-tools\nmise run build\nmise run devtools:prek-install\nmise run devtools:install-pre-push-hook' ]
}

@test "worktree setup checks trust before syncing Vale and uv" {
  root=$(git -C "$BATS_TEST_DIRNAME" rev-parse --show-toplevel)

  run mise -C "$root" task info devtools:worktree-setup --json

  [ "$status" -eq 0 ]
  script=$(jq -r '.run[]' <<<"$output")
  [[ "$script" == *'mise trust --show'* ]]
  [ "$(grep -Fc 'mise trust' <<<"$script")" -eq 2 ]
  [[ "$script" == *'echo "  mise trust"'* ]]
  [[ "$script" == *$'vale sync\nuv sync'* ]]
}
