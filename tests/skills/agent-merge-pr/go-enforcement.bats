#!/usr/bin/env bats

setup_file() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup_file
}

setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup
}

go_policy_inspect() {
  jq -Rn '[inputs | select(test("^[a-z_]+ = ")) | capture("^(?<key>[a-z_]+) = (?<value>.*)$")] | from_entries' \
    "$ROOT/.agents/agent-skills.conf" |
    uv run --quiet --script "$POLICY" inspect "$BASE_OID" "$HEAD_OID"
}

go_policy_evaluate() {
  go_policy_inspect | uv run --quiet --script "$POLICY" evaluate reviewer "$HEAD_OID" "$GH_FIXTURES/pr.json"
}

assert_go_enforcement() {
  local path=$1 body
  change "$path" baseline
  run go_policy_inspect
  [ "$status" -eq 0 ]
  jq -e --arg path "$path" '.findings == [[$path, "changes a golden file"]] and .owner_paths == []' <<<"$output"

  # These paths never join either owner tier, even when the base uses devtools policy.
  local key pattern
  for key in owner_paths constitutional_paths; do
    pattern=$(sed -n "s/^$key = //p" "$ROOT/.agents/agent-skills.conf")
    [[ ! "$path" =~ $pattern ]] || return 1
  done
  pr '.reviews=[]'
  run go_policy_evaluate
  [ "$status" -eq 0 ]
  jq -e '.owner_required == true' <<<"$output"

  pr ".reviews=[{author:{login:\"reviewer\",__typename:\"Bot\"},state:\"APPROVED\",commit:{oid:\"$HEAD_OID\"},submittedAt:\"2026-01-01T00:00:00Z\",lastEditedAt:null}]"
  for body in '' 'Loosens: invalid' $'Loosens: no\n\nLoosens: no' 'Loosens: yes'; do
    pr ".reviews[0].body=$(jq -n --arg body "Reviewed head: $HEAD_OID"$'\n\n'"$body" '$body')"
    run go_policy_evaluate
    [ "$status" -eq 0 ]
    jq -e '.owner_required == true and .owner_rule == "owner approval"' <<<"$output"
  done
  pr ".reviews[0].body=\"Reviewed head: $HEAD_OID\n\nLoosens: no\""
  run go_policy_evaluate
  [ "$status" -eq 0 ]
  jq -e '.owner_required == false and .owner_rule == "App review states Loosens: no" and .owner_paths == []' <<<"$output"
  jq -e '.labels == [] and (.reviews | length) == 1' "$GH_FIXTURES/pr.json"
}

@test "753 AC1 agentrepo enforcement package is golden only" {
  assert_go_enforcement internal/agentrepo/input.txt
}

@test "753 AC1 check gates package is golden only" {
  assert_go_enforcement internal/checks/input.txt
}

@test "753 AC1 machine authority configuration package is golden only" {
  assert_go_enforcement internal/config/input.txt
}

@test "753 AC1 merge policy package is golden only" {
  assert_go_enforcement internal/mergepolicy/input.txt
}

@test "753 AC1 review verdict package is golden only" {
  assert_go_enforcement internal/review/input.txt
}

@test "753 AC1 signing package is golden only" {
  assert_go_enforcement internal/signing/units/input.txt
}

@test "753 AC1 commit gates package is golden only" {
  assert_go_enforcement internal/commitlint/input.txt
}

@test "753 AC1 guarded posting package is golden only" {
  assert_go_enforcement internal/ghpost/input.txt
}

@test "753 AC1 agentrepo entry point is golden only" {
  assert_go_enforcement cmd/agentrepo/main.go
}

@test "753 AC1 signing entry point is golden only" {
  assert_go_enforcement cmd/ssh-sign/main.go
}

@test "753 AC1 commit gate entry point is golden only" {
  assert_go_enforcement cmd/commit-lint/main.go
}

@test "753 AC1 guarded posting entry point is golden only" {
  assert_go_enforcement cmd/gh-post/main.go
}

@test "753 AC1 subcommand registration is golden only" {
  assert_go_enforcement internal/agentrepo/command.go
}

@test "753 AC2 stale approval cannot waive new Go enforcement" {
  change internal/mergepolicy/approval.go baseline
  local old_head=$HEAD_OID
  change internal/mergepolicy/approval.go next
  pr ".reviews[0] += {body:\"Reviewed head: $old_head\n\nLoosens: no\",lastEditedAt:null} | .reviews[0].author.__typename=\"Bot\" | .reviews[0].commit.oid=\"$old_head\""
  run go_policy_evaluate
  [ "$status" -eq 0 ]
  jq -e '.owner_required == true and any(.findings[]; .[1] == "changes a golden file")' <<<"$output"
}

@test "753 AC2 deleted and renamed Go enforcement files retain golden findings" {
  change internal/mergepolicy/deleted.go baseline
  change internal/mergepolicy/renamed.go baseline
  change moved-in.go baseline
  BASE_OID=$HEAD_OID
  git rm --quiet internal/mergepolicy/deleted.go
  git mv internal/mergepolicy/renamed.go moved-out.go
  git mv moved-in.go internal/mergepolicy/moved-in.go
  git commit --quiet -m 'test: delete and rename Go enforcement'
  HEAD_OID=$(git rev-parse HEAD)
  pr ".headRefOid=\"$HEAD_OID\" | .reviews[0].commit.oid=\"$HEAD_OID\""
  run go_policy_evaluate
  [ "$status" -eq 0 ]
  jq -e '.owner_required == true and (.findings | sort) == ([
    ["internal/mergepolicy/deleted.go", "changes a golden file"],
    ["internal/mergepolicy/renamed.go", "changes a golden file"],
    ["internal/mergepolicy/moved-in.go", "changes a golden file"]] | sort)' <<<"$output"
}

@test "753 AC2 pure helpers and adjacent names remain outside protection" {
  local path key pattern
  for key in golden_paths owner_paths constitutional_paths; do
    pattern=$(sed -n "s/^$key = //p" "$ROOT/.agents/agent-skills.conf")
    for path in internal/buildmeta/meta.go internal/pins/path.go internal/tokens/format.go \
      internal/review-helper/format.go internal/mergepolicy.go cmd/agentrepo-helper/main.go \
      docs/internal/mergepolicy/approval.go; do
      [[ ! "$path" =~ $pattern ]] || { echo "$key matched $path"; return 1; }
    done
  done
}

@test "753 AC2 fixed legacy path classifications are unchanged" {
  local golden constitutional owner path expected actual
  golden=$(sed -n 's/^golden_paths = //p' "$ROOT/.agents/agent-skills.conf")
  constitutional=$(sed -n 's/^constitutional_paths = //p' "$ROOT/.agents/agent-skills.conf")
  owner=$(sed -n 's/^owner_paths = //p' "$ROOT/.agents/agent-skills.conf")
  while read -r expected path; do
    actual=
    if [[ "$path" =~ $golden ]]; then actual+=1; else actual+=0; fi
    if [[ "$path" =~ $constitutional ]]; then actual+=1; else actual+=0; fi
    if [[ "$path" =~ $owner ]]; then actual+=1; else actual+=0; fi
    [ "$actual" = "$expected" ] || { echo "$path: expected $expected, got $actual"; return 1; }
  done <<'PATHS'
111 .agents/agent-skills.conf
111 .agents/review-instructions.md
111 .github/rulesets/main.json
010 .github/CODEOWNERS
111 .claude/settings.json
111 packages/agents/source/lib/lib.sh
101 packages/agents/source/lib/agent.sh
010 packages/agents/source/lib/signing.sh
101 packages/agents/source/skills/commit/claude/scripts/guard-git.sh
101 packages/agents/source/skills/merge-pr/claude/scripts/guard-merge.sh
111 packages/agents/source/skills/merge-pr/shared/scripts/merge-policy.py
111 packages/agents/source/skills/merge-pr/shared/scripts/merge-agent.sh
111 packages/agents/claude/.apm/skills/merge-pr/scripts/lib.sh
101 packages/agents/codex/.apm/skills/commit/scripts/agent.sh
111 .claude/skills/agent-merge-pr/scripts/merge-policy.py
111 .agents/skills/agent-merge-pr/scripts/merge-agent.sh
000 .agents/agent-skills.conf.backup
000 .github/CODEOWNERS.backup
000 .claude/settings.json.backup
000 packages/agents/source/lib/other.sh
000 packages/agents/source/skills/pr/shared/scripts/create-pr.sh
000 packages/agents/source/skills/merge-pr/shared/scripts/squash-merge.sh
000 mise.toml
PATHS
}

@test "753 AC1 Go waiver retains check label and detected-loosening holds" {
  git checkout --quiet main
  cp "$ROOT/.agents/agent-skills.conf" .agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: configure devtools policy'
  BASE_OID=$(git rev-parse HEAD)
  base_at "$BASE_OID"
  git checkout --quiet topic
  git rebase --quiet main
  git push --quiet origin main
  change internal/mergepolicy/approval.go baseline
  pr ".reviews[0] += {body:\"Reviewed head: $HEAD_OID\n\nLoosens: no\",lastEditedAt:null} | .reviews[0].author.__typename=\"Bot\""
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -eq 0 ]

  pr '.statusCheckRollup[0].conclusion="SKIPPED"'
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" != *needs-owner* ]] || return 1
  pr '.statusCheckRollup[0].conclusion="SUCCESS" | .labels=[{name:"merge:owner-review"}]'
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"needs-owner: the PR has the owner-review label"* ]] || return 1

  pr '.labels=[]'
  change internal/mergepolicy/approval.go 'value // nolint'
  pr ".reviews[0].body=\"Reviewed head: $HEAD_OID\n\nLoosens: no\""
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *needs-owner* ]]
}
