#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_pr_setup
}

@test "AC1 refuses the owner-review label in create and update drafts" {
  grant
  for owner_label in owner-review merge:owner-review; do
    if [ "$owner_label" != owner-review ]; then
      printf 'owner_review_label = %s\n' "$owner_label" >.agents/agent-skills.conf
    fi
    export GH_LABELS=$'enhancement\nbug\n'"$owner_label"
    for open in '' 1; do
      export GH_OPEN_PR=$open
      write_draft
      bash "$SCRIPTS/review.sh"
      replace_text PR_AGENTDESC.md 'labels: [enhancement]' "labels: [enhancement, '$owner_label']"
      run bash "$SCRIPTS/publish.sh"
      [ "$status" -eq 1 ]
      [[ "$output" == *'applied only by the owner-approval route'*'approved at its head, green and mergeable'* ]]
      [ -z "$(remote_topic)" ]
      refute_grep '^pr create \|^pr edit ' "$GH_LOG"
    done
  done
}

@test "AC2 preflight offers labels without the configured or default owner-review label" {
  for owner_label in owner-review merge:owner-review; do
    if [ "$owner_label" != owner-review ]; then
      printf 'owner_review_label = %s\n' "$owner_label" >.agents/agent-skills.conf
    fi
    export GH_LABELS=$'enhancement\nbug\n'"$owner_label"
    rm PR_AGENTDESC.md
    run bash "$SCRIPTS/preflight.sh"
    [ "$status" -eq 0 ]
    choices=$(printf '%s\n' "$output" | sed -n '/^labels:$/,/^open milestones:$/p')
    [[ "$choices" == *enhancement* ]]
    [[ "$choices" != *"$owner_label"* ]]
    grep -qx 'labels: \[\]' PR_AGENTDESC.md
  done
}

@test "AC3 update preserves only the owner-review label omitted by the draft" {
  grant
  export GH_OPEN_PR=1
  for owner_label in owner-review merge:owner-review; do
    if [ "$owner_label" != owner-review ]; then
      printf 'owner_review_label = %s\n' "$owner_label" >.agents/agent-skills.conf
    fi
    export GH_PR_STATE='{"baseRefName":"main","isDraft":false,"labels":[{"name":"bug"},{"name":"'"$owner_label"'"},{"name":"'"$owner_label"'-extra"}],"reviewRequests":[{"login":"'"$owner_label"'"}],"assignees":[],"milestone":null}'
    : >"$GH_LOG"
    bash "$SCRIPTS/review.sh"
    run bash "$SCRIPTS/publish.sh"
    [ "$status" -eq 0 ]
    refute_grep "pr edit 1 --remove-label $owner_label$" "$GH_LOG"
    refute_grep "pr edit 1 --add-label $owner_label$" "$GH_LOG"
    grep -q 'pr edit 1 --remove-label bug$' "$GH_LOG"
    grep -q "pr edit 1 --remove-label $owner_label-extra$" "$GH_LOG"
    grep -q "pr edit 1 --remove-reviewer $owner_label$" "$GH_LOG"
  done
}

@test "AC4 a draft without the owner-review label publishes" {
  grant
  printf 'owner_review_label = merge:owner-review\n' >.agents/agent-skills.conf
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 0 ]
  grep -q 'pr create .*--label enhancement' "$GH_LOG"
  refute_grep 'pr create .*--label merge:owner-review' "$GH_LOG"
}
