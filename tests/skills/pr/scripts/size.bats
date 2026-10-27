#!/usr/bin/env bats

setup() {
  source "$BATS_TEST_DIRNAME/../test_helper.bash"
  pr_setup
  export DEVTOOLS_GH_LOGIN=agent
}

@test "both PR entry points refuse size before review or push with the draft base" {
  git branch stack main
  git update-ref refs/remotes/origin/stack main
  sed -i 's/base: main/base: stack/' PR_AGENTDESC.md
  export SIZE_MODE=over
  for mode in pr agent-pr; do
    entry="$ROOT/packages/agents/codex/.apm/skills/$mode/scripts"
    for step in preflight publish; do
      script=$step
      if [ "$mode/$step" = pr/publish ]; then script=create-pr; fi
      run bash "$entry/$script.sh"
      [ "$status" -ne 0 ]
      [[ "$output" == *'split by concern, along commit boundaries, or as a stacked series'* ]]
      [ -s PR_AGENTDESC.md ]
      [ -z "$(remote_topic)" ]
      refute_grep 'review' "$GATE_LOG"
    done
  done
  grep -q -- '--draft PR_AGENTDESC.md' "$SIZE_LOG"
}

@test "both PR entry points stop for an old binary's unknown subcommand" {
  export SIZE_MODE=old
  for mode in pr agent-pr; do
    entry="$ROOT/packages/agents/codex/.apm/skills/$mode/scripts"
    for step in preflight publish; do
      script=$step
      if [ "$mode/$step" = pr/publish ]; then script=create-pr; fi
      run bash "$entry/$script.sh"
      [ "$status" -ne 0 ]
      [[ "$output" == *'unknown command "check"'* ]]
      [ -z "$(remote_topic)" ]
      refute_grep 'review' "$GATE_LOG"
    done
  done
  grep -q '^size check --publish --base ' "$SIZE_LOG"
}

@test "supported binary receives the base at preflight and publication" {
  git branch stack main
  git update-ref refs/remotes/origin/stack main
  for mode in pr agent-pr; do
    entry="$ROOT/packages/agents/codex/.apm/skills/$mode/scripts"
    for step in preflight create-pr; do
      script=$step
      if [ "$mode/$step" = agent-pr/create-pr ]; then script=publish; fi
      write_draft
      sed -i 's/base: main/base: stack/' PR_AGENTDESC.md
      : >"$SIZE_LOG"
      if [ "$step" = preflight ]; then
        run bash "$entry/$script.sh"
        base=refs/remotes/origin/main
      else
        run bash "$entry/$script.sh" --check-description
        base=origin/stack
      fi
      [ "$status" -eq 0 ]
      grep -qx "size check --publish --base $base --draft PR_AGENTDESC.md" "$SIZE_LOG"
    done
  done
}
