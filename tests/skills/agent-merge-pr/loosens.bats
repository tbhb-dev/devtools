#!/usr/bin/env bats
setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup
}

app_no() {
  pr ".reviews[0] += {body:\"Reviewed head: $HEAD_OID\n\nLoosens: no\",lastEditedAt:null} | .reviews[0].author.__typename=\"Bot\""
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

@test "AC4 invalid headers and statements require owner" {
  change golden/result.txt baseline
  app_no
  local body header="Reviewed head: $HEAD_OID"
  for body in '' 'Loosens: no' "Reviewed head: invalid" $'Review.\n\nLoosens: no' \
    "$header"$'\nLoosens: yes' "$header"$'\nLoosens: NO' \
    "$header"$'\n Loosens: no' "$header"$'\nLoosens: no \t' \
    "$header"$'\nLoosens: no or yes' "$header"$'\nLoosens: no\n\nLoosens: no' \
    "$header"$'\nLoosens: no\n\nLoosens: yes' \
    "$header"$'\nModel: model\nModel: model\nLoosens: no' \
    "$header"$'\nModel: model/invalid\nLoosens: no' \
    "$header"$'\nLoosens: no\ntext' \
    "$header"$'\nLoosens: no\n---'; do
    pr ".reviews[0].body=$(jq -n --arg body "$body" '$body')"
    run evaluate_policy
    [ "$status" -eq 0 ]
    jq -e '.owner_required == true' <<<"$output"
  done
}

@test "AC4 mismatched plain and backticked header SHAs require owner" {
  change golden/result.txt baseline
  app_no
  local sha body failed=0
  for sha in '0000000000000000000000000000000000000000' '`0000000000000000000000000000000000000000`'; do
    body="Reviewed head: $sha"$'\n\nLoosens: no'
    pr ".reviews[0].body=$(jq -n --arg body "$body" '$body')"
    run evaluate_policy
    [ "$status" -eq 0 ]
    jq -e '.owner_required == true' <<<"$output" || failed=1
  done
  [ "$failed" -eq 0 ]
}

@test "AC4 plain header accepts optional SHA backticks Model and CRLF" {
  change golden/result.txt baseline
  app_no
  local sha model body
  for sha in "$HEAD_OID" "\`$HEAD_OID\`"; do
    for model in '' $'Model: codex gpt-6.1_sol\n\n'; do
      body="Reviewed head: $sha"$'\n\n'"$model"'Loosens: no'
      pr ".reviews[0].body=$(jq -n --arg body "$body" '$body')"
      run evaluate_policy
      [ "$status" -eq 0 ]
      jq -e '.owner_required == false' <<<"$output"
      pr '.reviews[0].body += "\n\n> Ignored review text\n\t```" | .reviews[0].body |= gsub("\n"; "\r\n")'
      run evaluate_policy
      [ "$status" -eq 0 ]
      jq -e '.owner_required == false' <<<"$output"
    done
  done
  pr ".reviews[0].body=\"Reviewed head: $HEAD_OID\nModel: model\nLoosens: no\n\""
  run evaluate_policy
  [ "$status" -eq 0 ]
  jq -e '.owner_required == false' <<<"$output"
}

@test "AC4 invalid characters anywhere in the body require owner" {
  change golden/result.txt baseline
  app_no
  local character
  for character in '\u00a0' '\u000c' '\u0000' '\r' '\u007f' '\u00e9'; do
    app_no
    pr ".reviews[0].body += \"\\n\\n$character\""
    run evaluate_policy
    [ "$status" -eq 0 ]
    jq -e '.owner_required == true' <<<"$output"
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
assert "Normalize carriage return plus line feed (`CRLF`) to line feed (`LF`) before checking the body." in text
assert "Every character in the whole body must belong to printable `ASCII` (`0x20` through `0x7E`), or be a tab or `LF`." in text
assert "Unicode whitespace, form feed, `NUL`, and an unpaired carriage return require the owner." in text
assert "The body must begin with `Reviewed head: <40 lowercase hex digits>`." in text
assert "Review authors may put the SHA inside one pair of backticks." in text
assert "The header SHA must equal the merge review's pinned head. A mismatch requires the owner." in text
assert "An optional `Model: <letters, digits, dot, dash, underscore, space>` line may follow." in text
assert "Empty lines may separate these header lines and the statement." in text
assert "The first line after this header block must be exactly `Loosens: no`. Surrounding whitespace or extra text requires the owner." in text
assert "The following line must be empty, or the body must end." in text
assert "Ignore later text except for the whole-body character check and duplicate statement check." in text
assert "The App review brief must place `Loosens: no` in this header position." in text
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

@test "AC7 bypass bodies from independent reviews require owner" {
  change golden/result.txt baseline
  app_no
  local body header="Reviewed head: $HEAD_OID"
  for body in $'> Loosens: no' \
    $'- > quoted\nLoosens: no' $'1. > quoted\nLoosens: no' \
    $'> quoted\nLoosens: no' $'`quoted\n\nLoosens: no\n`' \
    $'``quoted\n\nLoosens: no\n``' \
    $'> quoted\n\u00a0\nLoosens: no' $'- > quoted\n\u00a0\nLoosens: no' \
    $'1. > quoted\n\u00a0\nLoosens: no' \
    $'``quoted\n\u00a0\nLoosens: no\n``' \
    $'text <!--\n\u00a0\nLoosens: no\n-->' \
    $'> quoted\n\f\nLoosens: no' $'text <!--\n\nLoosens: no\n-->' \
    $'Loosens: no\n---' $'```text\nLoosens: no\n```' \
    $'~~~\nLoosens: no\n~~~' $'    Loosens: no' \
    $'Loosens: no trailing text' $'Loosens\uff1a no' \
    $'Loosens:\u00a0no'; do
    pr ".reviews[0].body=$(jq -n --arg body "$body" '$body')"
    run evaluate_policy
    [ "$status" -eq 0 ]
    jq -e '.owner_required == true' <<<"$output"
    body="$header"$'\n\n'"$body"
    pr ".reviews[0].body=$(jq -n --arg body "$body" '$body')"
    run evaluate_policy
    [ "$status" -eq 0 ]
    jq -e '.owner_required == true' <<<"$output"
  done
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

@test "AC1 constitutional_paths ^$ lets a PR edit agent-skills.conf on exact-head App approval" {
  git checkout --quiet main
  sed 's/^constitutional_paths = .*/constitutional_paths = ^$/' .agents/agent-skills.conf >.agents/agent-skills.conf.tmp
  mv .agents/agent-skills.conf.tmp .agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: set base constitutional_paths to ^$'
  BASE_OID=$(git rev-parse HEAD)
  base_at "$BASE_OID"
  git checkout --quiet topic
  git rebase --quiet main
  git push --quiet --force origin main topic
  change .agents/agent-skills.conf "$(sed 's|^owner_paths = .*|owner_paths = ^custom/|' .agents/agent-skills.conf)"
  app_no
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -eq 0 ]
  [[ "$output" != *needs-owner* ]] || return 1
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  grant merge
  run bash "$AGENT/merge.sh" 7
  [ "$status" -eq 0 ]
  [ -f "$GH_FIXTURES/merged" ]
}

@test "AC2 constitutional_paths matching agent-skills.conf holds the PR with needs-owner" {
  git checkout --quiet main
  sed 's|^constitutional_paths = .*|constitutional_paths = ^[.]agents/agent-skills[.]conf$|' .agents/agent-skills.conf >.agents/agent-skills.conf.tmp
  mv .agents/agent-skills.conf.tmp .agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: protect agent-skills.conf at base'
  BASE_OID=$(git rev-parse HEAD)
  base_at "$BASE_OID"
  git checkout --quiet topic
  git rebase --quiet main
  git push --quiet --force origin main topic
  change .agents/agent-skills.conf "$(sed 's|^owner_paths = .*|owner_paths = ^custom/|' .agents/agent-skills.conf)"
  app_no
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *needs-owner* ]] || return 1
  [[ "$output" == *'changes a constitutional file'* ]] || return 1
  [[ "$output" != *'changes merge policy'* ]] || return 1
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *needs-owner* ]]
}

@test "AC5 normal PR merges without constitutional_paths configured" {
  git checkout --quiet main
  sed '/^constitutional_paths = /d' .agents/agent-skills.conf >.agents/agent-skills.conf.tmp
  mv .agents/agent-skills.conf.tmp .agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: omit constitutional_paths'
  BASE_OID=$(git rev-parse HEAD)
  base_at "$BASE_OID"
  git checkout --quiet topic
  git rebase --quiet main
  HEAD_OID=$(git rev-parse HEAD)
  git push --quiet --force origin main topic
  pr ".headRefOid=\"$HEAD_OID\" | .reviews[0].commit.oid=\"$HEAD_OID\""
  app_no
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -eq 0 ]
  [[ "$output" != *"Policy pattern constitutional_paths must not be empty"* ]] || return 1
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  grant merge
  run bash "$AGENT/merge.sh" 7
  [ "$status" -eq 0 ]
  [ -f "$GH_FIXTURES/merged" ]
}

@test "AC5 PR adding constitutional_paths to agent-skills.conf merges on exact-head App approval" {
  git checkout --quiet main
  sed '/^constitutional_paths = /d' .agents/agent-skills.conf >.agents/agent-skills.conf.tmp
  mv .agents/agent-skills.conf.tmp .agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: omit constitutional_paths'
  BASE_OID=$(git rev-parse HEAD)
  base_at "$BASE_OID"
  git checkout --quiet topic
  git rebase --quiet main
  git push --quiet --force origin main topic
  change .agents/agent-skills.conf "$(cat .agents/agent-skills.conf)"$'\nconstitutional_paths = ^$'
  app_no
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -eq 0 ]
  [[ "$output" != *"Policy pattern constitutional_paths must not be empty"* ]] || return 1
  [[ "$output" != *needs-owner* ]] || return 1
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  grant merge
  run bash "$AGENT/merge.sh" 7
  [ "$status" -eq 0 ]
  [ -f "$GH_FIXTURES/merged" ]
}

@test "AC7 base constitutional_paths holds a PR that sets the key to ^$ or removes it" {
  git checkout --quiet main
  sed 's|^constitutional_paths = .*|constitutional_paths = ^[.]agents/agent-skills[.]conf$|' .agents/agent-skills.conf >.agents/agent-skills.conf.tmp
  mv .agents/agent-skills.conf.tmp .agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: protect agent-skills.conf at base'
  BASE_OID=$(git rev-parse HEAD)
  base_at "$BASE_OID"
  git checkout --quiet topic
  git rebase --quiet main
  git push --quiet --force origin main topic

  local base_config
  base_config=$(cat .agents/agent-skills.conf)
  change .agents/agent-skills.conf "$(printf '%s\n' "$base_config" | sed 's/^constitutional_paths = .*/constitutional_paths = ^$/; s|^owner_paths = .*|owner_paths = ^custom/|')"
  app_no
  run evaluate_policy
  [ "$status" -eq 0 ]
  jq -e '.owner_required == true and any(.findings[]; .[0] == ".agents/agent-skills.conf" and .[1] == "changes a constitutional file")' <<<"$output"
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *needs-owner* ]] || return 1
  [[ "$output" == *'changes a constitutional file'* ]] || return 1
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *needs-owner* ]] || return 1

  change .agents/agent-skills.conf "$(printf '%s\n' "$base_config" | sed '/^constitutional_paths = /d; s|^owner_paths = .*|owner_paths = ^other/|')"
  app_no
  run evaluate_policy
  [ "$status" -eq 0 ]
  jq -e '.owner_required == true and any(.findings[]; .[0] == ".agents/agent-skills.conf" and .[1] == "changes a constitutional file")' <<<"$output"
  run bash "$AGENT/preflight.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *needs-owner* ]] || return 1
  [[ "$output" == *'changes a constitutional file'* ]] || return 1
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *needs-owner* ]]
}
