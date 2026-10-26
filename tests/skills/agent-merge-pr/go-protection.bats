#!/usr/bin/env bats
setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup
  # Install the repository policy on the base, so only the replacement changes.
  git checkout --quiet main
  cp "$ROOT/.agents/agent-skills.conf" .agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: install repository policy'
  BASE_OID=$(git rev-parse HEAD)
  git checkout --quiet topic
  git rebase --quiet main
  base_at "$BASE_OID"
}

replacement_policy() {
  local key value config='{}'
  for key in golden_paths constitutional_paths owner_paths loosening_markers gate_config_paths disposable_ignored; do
    value=$(bash -c 'source "$1/lib.sh"; dt_repo_conf "$2"' _ "$SCRIPTS" "$key")
    config=$(jq --arg key "$key" --arg value "$value" '.[$key]=$value' <<<"$config")
  done
  uv run --quiet --script "$POLICY" inspect "$BASE_OID" "$HEAD_OID" <<<"$config"
}

assert_replacement() {
  local path=$1
  change "$path" '// enforcement'
  run replacement_policy
  [ "$status" -eq 0 ]
  jq -e --arg path "$path" '.owner_paths == [$path] and .findings == [[$path,"changes a golden file"]]' <<<"$output"
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *needs-owner* ]]
  [[ "$output" == *"changes a golden file"* ]]
  approve_owner
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  pr '.reviews |= map(select(.author.login != "owner"))'
  pr ".reviews[0] += {body:\"Reviewed head: $HEAD_OID\\n\\nLoosens: no\",lastEditedAt:null} | .reviews[0].author.__typename=\"Bot\""
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
}

@test "AC1 replacement command construction is golden" { assert_replacement internal/agentrepo/review.go; }
@test "AC1 replacement registration is golden" { assert_replacement internal/agentrepo/command.go; }
@test "AC1 replacement checks are golden" { assert_replacement internal/checks/checks.go; }
@test "AC1 replacement signing is golden" { assert_replacement internal/signing/signing.go; }
@test "AC1 replacement review is golden" { assert_replacement internal/review/stamp.go; }
@test "AC1 replacement configuration is golden" { assert_replacement internal/config/repository.go; }
@test "AC1 replacement commit enforcement is golden" { assert_replacement internal/commitlint/commit.go; }
@test "AC1 replacement posting enforcement is golden" { assert_replacement internal/ghpost/run.go; }
@test "AC1 replacement merge policy is golden" { assert_replacement internal/mergepolicy/policy.go; }
@test "AC1 replacement gates are golden" { assert_replacement internal/gates/gates.go; }
@test "AC1 replacement confirmation is golden" { assert_replacement internal/confirmation/confirm.go; }
@test "AC1 replacement delegation is golden" { assert_replacement internal/delegation/delegate.go; }
@test "AC1 agentrepo entry point is golden" { assert_replacement cmd/agentrepo/main.go; }
@test "AC1 signing entry point is golden" { assert_replacement cmd/ssh-sign/main.go; }
@test "AC1 commit entry point is golden" { assert_replacement cmd/commit-lint/main.go; }
@test "AC1 posting entry point is golden" { assert_replacement cmd/gh-post/main.go; }

@test "AC2 replacement push invalidates owner approval" {
  change internal/review/stamp.go '// first'
  approve_owner
  change internal/review/stamp.go '// second'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *needs-owner* ]]
  approve_owner
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
}

@test "AC2 replacement deletion and both rename names stay golden" {
  change internal/review/stamp.go '// first'
  BASE_OID=$HEAD_OID
  git mv internal/review/stamp.go helper.go
  git commit --quiet -m 'test: move replacement out'
  HEAD_OID=$(git rev-parse HEAD)
  run replacement_policy
  [ "$status" -eq 0 ]
  jq -e '.owner_paths == ["internal/review/stamp.go"] and .findings == [["internal/review/stamp.go","changes a golden file"]]' <<<"$output"
  BASE_OID=$HEAD_OID
  mkdir -p internal/review
  git mv helper.go internal/review/stamp.go
  git commit --quiet -m 'test: move replacement in'
  HEAD_OID=$(git rev-parse HEAD)
  run replacement_policy
  [ "$status" -eq 0 ]
  jq -e '.owner_paths == ["internal/review/stamp.go"] and .findings == [["internal/review/stamp.go","changes a golden file"]]' <<<"$output"
  BASE_OID=$HEAD_OID
  git rm --quiet internal/review/stamp.go
  git commit --quiet -m 'test: delete replacement'
  HEAD_OID=$(git rev-parse HEAD)
  run replacement_policy
  [ "$status" -eq 0 ]
  jq -e '.owner_paths == ["internal/review/stamp.go"] and .findings == [["internal/review/stamp.go","changes a golden file"]]' <<<"$output"
}

@test "AC2 existing pattern alternatives are preserved and helpers excluded" {
  local key old current path
  for key in golden_paths constitutional_paths owner_paths; do
    old=$(git -C "$ROOT" show origin/main:.agents/agent-skills.conf | sed -n "s/^$key = //p")
    current=$(sed -n "s/^$key = //p" "$ROOT/.agents/agent-skills.conf")
    # The old expression is kept byte for byte as the first alternative.
    [[ "$current" == "$old" || "$current" == "$old"'|^('* ]]
    for path in .agents/agent-skills.conf .agents/review-instructions.md .github/rulesets/main.json \
      .github/CODEOWNERS .claude/settings.json packages/agents/source/lib/lib.sh \
      packages/agents/source/lib/agent.sh packages/agents/source/lib/signing.sh \
      packages/agents/source/skills/merge-pr/shared/scripts/merge-policy.py \
      .agents/skills/merge-pr/scripts/merge-agent.sh; do
      if [[ "$path" =~ $old ]]; then
        [[ "$path" =~ $current ]]
      fi
    done
    for path in internal/markdown/format.go internal/buildmeta/version.go internal/hookio/path.go \
      internal/checks-extra/check.go internal/mergepolicy.go cmd/agentcontext/main.go \
      cmd/agentrepo-extra/main.go; do
      [[ ! "$path" =~ $current ]] || return 1
    done
  done
}
