#!/usr/bin/env bats

@test "AC8 full-suite test reproducer retains CI coverage and offers affected rerun (#648)" {
  script="$BATS_TEST_DIRNAME/../../../../packages/agents/source/skills/fix-pr/shared/scripts/diagnose.sh"
  reproducer=$(sed -n '/^reproducer() {$/,/^}$/p' "$script")
  run bash -c "$reproducer"$'\nreproducer "Test (macos)"'
  [ "$status" -eq 0 ]
  [[ "$output" == *"mise run test"* ]]
  [[ "$output" == *"if changed paths cover the failure: mise run test-affected --base <merge-base>"* ]]
}
