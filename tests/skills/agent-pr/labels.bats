#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_pr_setup
}

@test "AC1 missing labels refuse with every missing name and available set" {
  replace_text PR_AGENTDESC.md 'labels: [enhancement]' 'labels: [change:test, missing, bug]'
  run bash "$SCRIPTS/check.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *'no label named "change:test"'* ]]
  [[ "$output" == *'no label named "missing"'* ]]
  [[ "$output" == *'Available labels:'*$'enhancement\nbug'* ]]
  [ ! -s "$GATE_LOG" ]
}

@test "AC2 existing labels pass without additional output" {
  replace_text PR_AGENTDESC.md 'labels: [enhancement]' 'labels: [enhancement, bug]'
  run bash "$SCRIPTS/check.sh"
  [ "$status" -eq 0 ]
  [ "$output" = "gates: bash -c mise run lint-pr-description" ]
}

@test "AC3 empty labels retain the required-list refusal without reading labels" {
  replace_text PR_AGENTDESC.md 'labels: [enhancement]' 'labels: []'
  run bash "$SCRIPTS/check.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *'labels-empty  the pull request carries no label; preflight printed the repository'* ]]
  refute_grep '^label list ' "$GH_LOG"
}

@test "AC3 missing labels retain the required-key refusal without reading labels" {
  replace_text PR_AGENTDESC.md 'labels: [enhancement]' ''
  run bash "$SCRIPTS/check.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *'frontmatter-missing-key  the frontmatter sets no "labels"'* ]]
  refute_grep '^label list ' "$GH_LOG"
}

@test "AC4 unreadable label set refuses before lint" {
  export GH_LABEL_ERROR='label service unavailable'
  run bash "$SCRIPTS/check.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *'repository label set could not be read'* ]]
  [ ! -s "$GATE_LOG" ]
}

@test "AC5 label set is read once for multiple draft labels" {
  replace_text PR_AGENTDESC.md 'labels: [enhancement]' 'labels: [enhancement, bug]'
  bash "$SCRIPTS/check.sh"
  [ "$(grep -c '^label list ' "$GH_LOG")" -eq 1 ]
}
