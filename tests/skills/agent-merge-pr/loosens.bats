#!/usr/bin/env bats
setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup
}

app_no() {
  pr '.reviews[0] += {body:"Loosens: no",lastEditedAt:null} | .reviews[0].author.__typename="Bot"'
}

evaluate_policy() {
  inspect_policy | uv run --quiet --script "$POLICY" evaluate reviewer "$HEAD_OID" "$GH_FIXTURES/pr.json"
}

@test "AC1 golden findings alone accept the required App exact-head approval" {
  change golden/result.txt baseline
  app_no
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  run evaluate_policy
  [ "$status" -eq 0 ]
  jq -e '.owner_required == false and .owner_rule == "App review states Loosens: no"' <<<"$output"
}

@test "AC1 App waiver retains required checks and owner-review label" {
  change golden/result.txt baseline
  app_no
  pr '.statusCheckRollup[0].conclusion="SKIPPED"'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" != *needs-owner* ]]
  pr '.statusCheckRollup[0].conclusion="SUCCESS" | .labels=[{name:"owner-review"}]'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"needs-owner: the PR has the owner-review label"* ]]
}

@test "AC2 detected loosening overrides the App statement" {
  change golden/result.txt baseline
  change lint.yaml 'ignore: [rule]'
  app_no
  run evaluate_policy
  [ "$status" -eq 0 ]
  jq -e '.owner_required == true and .owner_rule == "owner approval"' <<<"$output"
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *needs-owner* ]]
}

@test "AC3 constitutional paths require owner even outside the golden set" {
  change constitution.txt baseline
  app_no
  run evaluate_policy
  [ "$status" -eq 0 ]
  jq -e '.owner_required == true and any(.findings[]; .[1] == "changes a constitutional file")' <<<"$output"
}

@test "AC3 constitutional key is accepted and duplicate and unknown keys are rejected" {
  sed '/^constitutional_paths = /d' .agents/agent-skills.conf >.agents/agent-skills.conf.tmp
  mv .agents/agent-skills.conf.tmp .agents/agent-skills.conf
  printf '\nconstitutional_paths = ^constitution.txt$\n' >>.agents/agent-skills.conf
  run bash -c 'source "$1/lib.sh"; dt_repo_conf constitutional_paths' _ "$SCRIPTS"
  [ "$status" -eq 0 ]
  printf '\nconstitutional_paths = ^other$\n' >>.agents/agent-skills.conf
  run bash -c 'source "$1/lib.sh"; dt_repo_conf constitutional_paths' _ "$SCRIPTS"
  [ "$status" -ne 0 ]
  [[ "$output" == *"Invalid or duplicate repository setting: constitutional_paths"* ]]
  printf '\nunknown_policy = ^$\n' >>.agents/agent-skills.conf
  run bash -c 'source "$1/lib.sh"; dt_repo_conf golden_paths' _ "$SCRIPTS"
  [ "$status" -ne 0 ]
  [[ "$output" == *"Invalid or duplicate repository setting: unknown_policy"* ]]
}

@test "AC3 devtools constitutional pattern covers authority sources and generated copies" {
  cp "$ROOT/.agents/agent-skills.conf" .agents/agent-skills.conf
  local pattern path
  run bash -c 'source "$1/lib.sh"; dt_repo_conf constitutional_paths' _ "$SCRIPTS"
  [ "$status" -eq 0 ]
  pattern=$output
  [ -n "$pattern" ]
  for path in .agents/agent-skills.conf .agents/review-instructions.md .github/rulesets/main.json .github/CODEOWNERS .claude/settings.json; do
    [[ "$path" =~ $pattern ]] || { echo "missed $path"; return 1; }
  done
  while IFS= read -r path; do
    [[ "$path" =~ $pattern ]] || { echo "missed $path"; return 1; }
  done < <(cd "$ROOT" && git ls-files packages/agents/source packages/agents/claude/.apm/skills packages/agents/codex/.apm/skills .claude/skills .agents/skills | rg '/(merge-policy.py|merge-agent.sh|signing.sh|lib.sh)$')
  for path in docs/agent-skills-migration.md .github/CODEOWNERS.backup packages/agents/source/lib/other.sh; do
    [[ ! "$path" =~ $pattern ]] || { echo "unexpected match $path"; return 1; }
  done
}

@test "AC4 missing ambiguous duplicate and non-no statements require owner" {
  change golden/result.txt baseline
  app_no
  local body
  for body in '' 'Loosens: yes' 'Loosens: NO' ' Loosens: no' 'Loosens: no or yes' $'Loosens: no\nLoosens: no'; do
    pr ".reviews[0].body=$(jq -n --arg body "$body" '$body')"
    run evaluate_policy
    [ "$status" -eq 0 ]
    jq -e '.owner_required == true' <<<"$output"
  done
}

@test "AC4 standalone paragraph accepts trailing whitespace and closed code" {
  change golden/result.txt baseline
  app_no
  local body
  for body in $'Review.\n\nLoosens: no' $'Loosens: no \t' $'`closed`\n\nLoosens: no' $'```\n`code\n```\n\nLoosens: no'; do
    pr ".reviews[0].body=$(jq -n --arg body "$body" '$body')"
    run evaluate_policy
    [ "$status" -eq 0 ]
    jq -e '.owner_required == false' <<<"$output"
  done
}

@test "AC4 only the latest decisive required App approval can waive owner" {
  change golden/result.txt baseline
  app_no
  local mutation
  for mutation in '.reviews[0].author.__typename="User"' '.reviews[0].author.login="other"' '.reviews[0].state="CHANGES_REQUESTED"' '.reviews[0].commit.oid="old"' 'del(.reviews[0].lastEditedAt)' '.reviews += [.reviews[0] + {state:"DISMISSED",submittedAt:"2026-01-02T00:00:00Z"}]'; do
    app_no
    pr "$mutation"
    run evaluate_policy
    [ "$status" -eq 0 ]
    jq -e '.owner_required == true' <<<"$output"
    pr ".reviews=[{author:{login:\"reviewer\"},state:\"APPROVED\",commit:{oid:\"$HEAD_OID\"},submittedAt:\"2026-01-01T00:00:00Z\"}]"
  done
}

@test "AC5 merge docs define the exact statement and fail-closed rule" {
  run python3 - "$ROOT/docs/agent-skills-migration.md" <<'PY'
import pathlib
import sys
text = pathlib.Path(sys.argv[1]).read_text()
assert "Loosens: yes|no" in text
assert "The line must start at column 0 with exactly `Loosens: no`, allowing trailing whitespace." in text
assert "It must be the first line or follow a blank line." in text
assert "The statement must lie outside fenced code blocks." in text
assert "The text before it must have an even number of backtick characters outside fences." in text
assert "Missing, duplicated, or ambiguous statements fail closed and require the owner." in text
assert "constitutional_paths" in text
assert "quoted" in text and "fenced" in text and "edited" in text
PY
  [ "$status" -eq 0 ]
}

@test "AC6 merge report records the App rule" {
  change golden/result.txt baseline
  app_no
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  grant merge
  run bash "$AGENT/merge.sh" 7
  [ "$status" -eq 0 ]
  [[ "$output" == *"App review states Loosens: no"* ]]
  jq -e '.owner_rule == "App review states Loosens: no" and .owner_override == ""' "$(git rev-parse --absolute-git-dir)/agent-skills/merge.result.json"
}

@test "AC6 merge report records owner approval rule" {
  change golden/result.txt baseline
  approve_owner
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  grant merge
  run bash "$AGENT/merge.sh" 7
  [ "$status" -eq 0 ]
  [[ "$output" == *"owner approval"* ]]
  jq -e '.owner_rule == "owner approval" and .owner_override == "owner"' "$(git rev-parse --absolute-git-dir)/agent-skills/merge.result.json"
}

@test "AC7 quoted statement cannot waive owner" {
  change golden/result.txt baseline
  app_no
  pr '.reviews[0].body="> Loosens: no"'
  run evaluate_policy
  [ "$status" -eq 0 ]
  jq -e '.owner_required == true' <<<"$output"
}

@test "AC7 fenced statement cannot waive owner" {
  change golden/result.txt baseline
  app_no
  local body
  for body in $'```text\nLoosens: no\n```' $'~~~\nLoosens: no\n~~~' $'    Loosens: no' $'> quote\nLoosens: no' $'<pre>\n\nLoosens: no\n</pre>' $'```\n```invalid\nLoosens: no\n```'; do
    pr ".reviews[0].body=$(jq -n --arg body "$body" '$body')"
    run evaluate_policy
    [ "$status" -eq 0 ]
    jq -e '.owner_required == true' <<<"$output"
  done
}

@test "AC7 list quotes and lazy quote continuations require owner" {
  change golden/result.txt baseline
  app_no
  local body
  for body in $'- > quoted\nLoosens: no' $'1. > quoted\nLoosens: no' $'> quoted\nLoosens: no'; do
    pr ".reviews[0].body=$(jq -n --arg body "$body" '$body')"
    run evaluate_policy
    [ "$status" -eq 0 ]
    jq -e '.owner_required == true' <<<"$output"
  done
}

@test "AC7 multiline inline code cannot waive owner" {
  change golden/result.txt baseline
  app_no
  pr '.reviews[0].body="`quoted\n\nLoosens: no\n`"'
  run evaluate_policy
  [ "$status" -eq 0 ]
  jq -e '.owner_required == true' <<<"$output"
}

@test "AC7 review edited after head moved cannot waive owner" {
  change golden/result.txt baseline
  app_no
  local old_head=$HEAD_OID
  change a.txt next
  pr ".reviews[0].commit.oid=\"$old_head\" | .reviews[0].lastEditedAt=\"2026-01-03T00:00:00Z\""
  run evaluate_policy
  [ "$status" -eq 0 ]
  jq -e '.owner_required == true' <<<"$output"
  pr ".reviews[0].commit.oid=\"$HEAD_OID\""
  run evaluate_policy
  [ "$status" -eq 0 ]
  jq -e '.owner_required == true' <<<"$output"
}

@test "AC7 live review edit restores owner requirement before merge" {
  change golden/result.txt baseline
  app_no
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  pr '.reviews[0].body="Loosens: yes" | .reviews[0].lastEditedAt="2026-01-03T00:00:00Z"'
  grant merge
  run bash "$AGENT/merge.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *needs-owner* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}
