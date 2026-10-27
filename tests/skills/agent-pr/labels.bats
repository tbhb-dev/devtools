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
  refute_grep 'labels(first:100' "$GH_LOG"
}

@test "AC3 missing labels retain the required-key refusal without reading labels" {
  replace_text PR_AGENTDESC.md 'labels: [enhancement]' ''
  run bash "$SCRIPTS/check.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *'frontmatter-missing-key  the frontmatter sets no "labels"'* ]]
  refute_grep 'labels(first:100' "$GH_LOG"
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
  [ "$(grep -c 'labels(first:100' "$GH_LOG")" -eq 1 ]
}

@test "supported binary without pr-labels retains the frozen shell label check" {
  export _DEVTOOLS_AGENTREPO_BIN="${BATS_TEST_TMPDIR}/bin/agentrepo-old"
  export _DEVTOOLS_AGENTREPO_CONFIG=false
  cat >"$_DEVTOOLS_AGENTREPO_BIN" <<'EOF'
#!/usr/bin/env bash
case "$1" in
--help) printf 'Available Commands:\n  config  Read configuration\n' ;;
*) exit 1 ;;
esac
EOF
  chmod +x "$_DEVTOOLS_AGENTREPO_BIN"
  run bash "$ROOT/packages/agents/codex/.apm/skills/pr/scripts/create-pr.sh" --check-description
  [ "$status" -eq 0 ]
  [ "$(cat "$GATE_LOG")" = lint-pr-description ]
  [ "$(grep -c 'labels(first:100' "$GH_LOG")" -eq 1 ]

  replace_text PR_AGENTDESC.md 'labels: [enhancement]' 'labels: [missing]'
  : >"$GATE_LOG"
  : >"$GH_LOG"
  run bash "$ROOT/packages/agents/codex/.apm/skills/pr/scripts/create-pr.sh" --check-description
  [ "$status" -ne 0 ]
  [[ "$output" == *'no label named "missing" in this repository. Preflight printed the label set.'* ]]
  [[ "$output" != *'Available labels:'* ]]
  [ "$(cat "$GATE_LOG")" = lint-pr-description ]
  [ "$(grep -c 'labels(first:100' "$GH_LOG")" -eq 1 ]
}

@test "absent binary retains the frozen shell label check" {
  export _DEVTOOLS_AGENTREPO_BIN=''
  export _DEVTOOLS_AGENTREPO_CONFIG=false
  run bash "$ROOT/packages/agents/codex/.apm/skills/pr/scripts/create-pr.sh" --check-description
  [ "$status" -eq 0 ]

  replace_text PR_AGENTDESC.md 'labels: [enhancement]' 'labels: [missing]'
  : >"$GATE_LOG"
  : >"$GH_LOG"
  run bash "$ROOT/packages/agents/codex/.apm/skills/pr/scripts/create-pr.sh" --check-description
  [ "$status" -ne 0 ]
  [[ "$output" == *'no label named "missing" in this repository. Preflight printed the label set.'* ]]
  [[ "$output" != *'Available labels:'* ]]
  [ "$(cat "$GATE_LOG")" = lint-pr-description ]
  [ "$(grep -c 'labels(first:100' "$GH_LOG")" -eq 1 ]
}

@test "matching binary rejects unknown labels before the shell property check" {
  replace_text PR_AGENTDESC.md 'labels: [enhancement]' 'labels: [missing]'
  run bash "$ROOT/packages/agents/codex/.apm/skills/pr/scripts/create-pr.sh" --check-description
  [ "$status" -ne 0 ]
  [[ "$output" == *'no label named "missing" in this repository'* ]]
  [[ "$output" == *'Available labels:'*$'enhancement\nbug'* ]]
  [ ! -s "$GATE_LOG" ]
  [ "$(grep -c 'labels(first:100' "$GH_LOG")" -eq 1 ]
}
