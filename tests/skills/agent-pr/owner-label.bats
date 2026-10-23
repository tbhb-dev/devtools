#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_pr_setup
}

@test "AC1 refuses the owner-review label in create and update drafts" {
  grant
  for owner_label in owner-review merge:owner-review MERGE:OWNER-REVIEW; do
    if [ "$owner_label" != owner-review ]; then
      printf 'owner_review_label = %s\n' "$owner_label" >.agents/agent-skills.conf
    fi
    api_label=$(printf '%s' "$owner_label" | tr '[:lower:]' '[:upper:]')
    if [ "$owner_label" = MERGE:OWNER-REVIEW ]; then api_label=merge:owner-review; fi
    export GH_LABELS=$'enhancement\nbug\n'"$api_label"
    for open in '' 1; do
      export GH_OPEN_PR=$open
      write_draft
      bash "$SCRIPTS/review.sh"
      replace_text PR_AGENTDESC.md 'labels: [enhancement]' "labels: [enhancement, '$api_label']"
      run bash "$SCRIPTS/publish.sh"
      [ "$status" -eq 1 ]
      [[ "$output" == *'applied only by the owner-approval route'*'approved at its head, green and mergeable'* ]]
      [ -z "$(remote_topic)" ]
      refute_grep '^pr create \|^pr edit ' "$GH_LOG"
    done
  done
}

@test "AC2 preflight offers labels without the configured or default owner-review label" {
  for owner_label in owner-review merge:owner-review MERGE:OWNER-REVIEW; do
    if [ "$owner_label" != owner-review ]; then
      printf 'owner_review_label = %s\n' "$owner_label" >.agents/agent-skills.conf
    fi
    api_label=$(printf '%s' "$owner_label" | tr '[:lower:]' '[:upper:]')
    if [ "$owner_label" = MERGE:OWNER-REVIEW ]; then api_label=merge:owner-review; fi
    export GH_LABELS=$'enhancement\nbug\n'"$api_label"
    rm PR_AGENTDESC.md
    run bash "$SCRIPTS/preflight.sh"
    [ "$status" -eq 0 ]
    choices=$(printf '%s\n' "$output" | sed -n '/^labels:$/,/^open milestones:$/p')
    [[ "$choices" == *enhancement* ]]
    [[ "$choices" != *"$api_label"* ]]
    grep -qx 'labels: \[\]' PR_AGENTDESC.md
  done
}

@test "AC3 update preserves only the owner-review label omitted by the draft" {
  grant
  export GH_OPEN_PR=1
  for owner_label in owner-review merge:owner-review MERGE:OWNER-REVIEW; do
    if [ "$owner_label" != owner-review ]; then
      printf 'owner_review_label = %s\n' "$owner_label" >.agents/agent-skills.conf
    fi
    api_label=$(printf '%s' "$owner_label" | tr '[:lower:]' '[:upper:]')
    if [ "$owner_label" = MERGE:OWNER-REVIEW ]; then api_label=merge:owner-review; fi
    export GH_PR_STATE='{"baseRefName":"main","isDraft":false,"labels":[{"name":"bug"},{"name":"'"$api_label"'"},{"name":"'"$api_label"'-extra"}],"reviewRequests":[{"login":"'"$api_label"'"}],"assignees":[],"milestone":null}'
    : >"$GH_LOG"
    bash "$SCRIPTS/review.sh"
    run bash "$SCRIPTS/publish.sh"
    [ "$status" -eq 0 ]
    refute_grep "pr edit 1 --remove-label $api_label$" "$GH_LOG"
    refute_grep "pr edit 1 --add-label $api_label$" "$GH_LOG"
    grep -q 'pr edit 1 --remove-label bug$' "$GH_LOG"
    grep -q "pr edit 1 --remove-label $api_label-extra$" "$GH_LOG"
    grep -q "pr edit 1 --remove-reviewer $api_label$" "$GH_LOG"
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

@test "duplicate frontmatter keys refuse create and update before review and publication" {
  grant
  export GH_LABELS=$'enhancement\nowner-review'
  for open in '' 1; do
    export GH_OPEN_PR=$open
    for duplicate in $'labels: [enhancement, owner-review]\nlabels: [enhancement]' $'labels: [enhancement]\nlabels: [enhancement, owner-review]' $'draft: false\ndraft: true'; do
      write_draft
      case "$duplicate" in
      labels:*) replace_text PR_AGENTDESC.md 'labels: [enhancement]' "$duplicate" ;;
      *) replace_text PR_AGENTDESC.md 'draft: false' "$duplicate" ;;
      esac
      run bash "$SCRIPTS/check.sh"
      [ "$status" -eq 1 ]
      [[ "$output" == *frontmatter-duplicate-key* ]]
      run bash "$SCRIPTS/review.sh"
      [ "$status" -eq 1 ]
      [[ "$output" == *frontmatter-duplicate-key* ]]
      [ ! -e "$(stamp_file)" ]
      run bash "$SCRIPTS/publish.sh"
      [ "$status" -eq 1 ]
      [[ "$output" == *frontmatter-duplicate-key* ]]
      [ -z "$(remote_topic)" ]
      refute_grep '^pr create \|^pr edit ' "$GH_LOG"
    done
  done
}
