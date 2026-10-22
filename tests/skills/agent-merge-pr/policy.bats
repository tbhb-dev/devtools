#!/usr/bin/env bats
# cspell:words ignorecase noqa
setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup
}

@test "App review uses GraphQL author type and login" {
  export DEVTOOLS_REQUIRED_REVIEWER=review-app DEVTOOLS_REQUIRED_REVIEWER_TYPE=Bot
  pr '.reviews[0].author={__typename:"Bot",login:"review-app"}'
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  grep -q 'author{__typename login}' "$GH_FIXTURES/gh.log"
}

@test "User review cannot satisfy required Bot reviewer" {
  export DEVTOOLS_REQUIRED_REVIEWER_TYPE=Bot
  pr '.reviews[0].author.__typename="User"'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *WAITING_REVIEW* ]]
}

@test "Bot review cannot satisfy required User reviewer" {
  export DEVTOOLS_REQUIRED_REVIEWER_TYPE=User
  pr '.reviews[0].author.__typename="Bot"'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *WAITING_REVIEW* ]]
}

@test "User reviewer passes when its configured type matches" {
  export DEVTOOLS_REQUIRED_REVIEWER_TYPE=User
  pr '.reviews[0].author.__typename="User"'
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
}

@test "typed reviewer fails closed when its author type is missing" {
  export DEVTOOLS_REQUIRED_REVIEWER_TYPE=Bot
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *WAITING_REVIEW* ]]
}

@test "review author snapshot refuses a changed head" {
  pr '.reviewHead="4444444444444444444444444444444444444444"'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"PR head changed while reading review authors"* ]]
}

@test "invalid reviewer type is rejected" {
  export DEVTOOLS_REQUIRED_REVIEWER_TYPE=App
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"required_reviewer_type must be Bot or User"* ]]
}

@test "REST bot reviewer login names the GraphQL form" {
  export DEVTOOLS_REQUIRED_REVIEWER='review-app[bot]'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"required_reviewer must use the GraphQL login review-app"* ]]
}

@test "REST bot owner login names the GraphQL form" {
  change golden/result.txt baseline
  approve_owner
  export DEVTOOLS_MERGE_OWNER='owner-app[bot]'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"merge_owner must use the GraphQL login owner-app"* ]]
}

@test "owner approval uses its own login independently of required reviewer type" {
  change golden/result.txt baseline
  approve_owner
  export DEVTOOLS_REQUIRED_REVIEWER_TYPE=Bot
  pr '.reviews[0].author.__typename="Bot" | .reviews[1].author.__typename="User"'
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
}

@test "golden file needs current machine owner approval and reports override" {
  change golden/result.txt baseline
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *needs-owner* ]]
  approve_owner
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
  grant merge
  run bash "$AGENT/merge.sh" 7
  [ "$status" -eq 0 ]
  [[ "$output" == *"Owner override: owner"* ]]
  jq -e '.owner_override=="owner"' "$(git rev-parse --absolute-git-dir)/agent-skills/merge.result.json"
}

@test "a push invalidates owner approval and a later decisive review wins" {
  change golden/result.txt baseline
  approve_owner
  change a.txt next
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *needs-owner* ]]
  pr ".reviews[1].commit.oid=\"$HEAD_OID\" | .reviews += [{author:{login:\"owner\"},state:\"CHANGES_REQUESTED\",commit:{oid:\"$HEAD_OID\"},submittedAt:\"2026-01-03T00:00:00Z\"}]"
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
}

@test "repository configuration cannot select merge owner or required reviewer" {
  printf '\nmerge_owner = attacker\n' >>.agents/agent-skills.conf
  run bash -c 'source "$1/lib.sh"; dt_repo_conf golden_paths' _ "$SCRIPTS"
  [ "$status" -ne 0 ]
  [[ "$output" == *"Invalid or duplicate"* ]]
}

@test "loosened gates need the same exact head owner override" {
  change lint.yaml 'ignore: [rule]'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *needs-owner* ]]
  approve_owner
  run bash "$AGENT/review.sh" 7
  [ "$status" -eq 0 ]
}

@test "scan ignores comments ordinary keys and banned-term values" {
  change lint.yaml $'ignorecase: true\n# vale off is documented here\nterms:\n  - allow this term\nmessage: ignore this word\nquoted: "a value # noqa"'
  run inspect_policy
  [ "$status" -eq 0 ]
  [ "$(printf '%s' "$output" | jq '.findings|length')" -eq 0 ]
}

@test "scan detects exclusion keys inline suppressions and removed hooks" {
  change lint.yaml $'ignore: [rule]\nvalue: 1 # noqa\n'
  run inspect_policy
  [ "$status" -eq 0 ]
  [ "$(printf '%s' "$output" | jq '.findings|length')" -ge 2 ]
}

@test "policy reader runs offline without third-party packages" {
  local cache="$BATS_TEST_TMPDIR/empty-uv-cache"
  mkdir -p "$cache"
  run env UV_CACHE_DIR="$cache" UV_OFFLINE=1 uv run --quiet --script "$POLICY" inspect "$BASE_OID" "$HEAD_OID" <<<"$(policy_config)"
  [ "$status" -eq 0 ]
}

@test "APM remote deployed paths are skipped while local overrides are scanned" {
  git checkout --quiet main
  cat >apm.lock.yaml <<'LOCK'
dependencies:
- repo_url: example/tooling
  deployed_files:
  - vendor/skills
- repo_url: _local/custom
  source: local
  deployed_files: [vendor/skills/custom]
LOCK
  git add -- apm.lock.yaml
  git commit --quiet -m 'test: deployed ownership'
  BASE_OID=$(git rev-parse HEAD)
  git checkout --quiet topic
  git rebase --quiet main
  change vendor/skills/upstream/lint.yaml 'ignore: [rule]'
  run inspect_policy
  [ "$status" -eq 0 ]
  [ "$(printf '%s' "$output" | jq '.findings|length')" -eq 0 ]
  change vendor/skills/custom/lint.yaml 'ignore: [rule]'
  run inspect_policy
  [ "$status" -eq 0 ]
  [ "$(printf '%s' "$output" | jq '.findings|length')" -eq 1 ]
}

@test "PR cannot exempt itself by editing its APM lockfile" {
  change apm.lock.yaml $'dependencies:\n- repo_url: example/remote\n  deployed_files: [lint.yaml]'
  change lint.yaml 'ignore: [rule]'
  run inspect_policy
  [ "$status" -eq 0 ]
  [ "$(printf '%s' "$output" | jq '.findings|length')" -eq 1 ]
}

@test "malformed base lockfile and invalid patterns fail closed" {
  run bash -c 'echo "{}" | uv run --quiet --script "$1" inspect "$2" "$3"' _ "$POLICY" "$BASE_OID" "$HEAD_OID"
  [ "$status" -ne 0 ]
  git checkout --quiet main
  echo 'dependencies: [' >apm.lock.yaml
  git add -- apm.lock.yaml
  git commit --quiet -m 'test: invalid lock'
  BASE_OID=$(git rev-parse HEAD)
  run inspect_policy
  [ "$status" -ne 0 ]
}

@test "deleted golden files and both rename names remain protected" {
  change golden/old.txt baseline
  BASE_OID=$HEAD_OID
  git mv golden/old.txt moved.txt
  git commit --quiet -m 'test: move baseline'
  HEAD_OID=$(git rev-parse HEAD)
  run inspect_policy
  [ "$status" -eq 0 ]
  [[ "$output" == *golden/old.txt* ]]
}

@test "merge policy preserves POSIX classes and newline paths" {
  change $'owned/line\nname' content
  run bash -c 'jq -n '\''{golden_paths:"^$",loosening_markers:"nolint",gate_config_paths:"^$",owner_paths:"^owned/line[[:space:]]name$",disposable_ignored:"^cache/"}'\'' | uv run --quiet --script "$1" inspect "$2" "$3"' _ "$POLICY" "$BASE_OID" "$HEAD_OID"
  [ "$status" -eq 0 ]
  [ "$(printf '%s' "$output" | jq '.owner_paths|length')" -eq 1 ]
}

@test "APM active owner resolves overlapping local and remote deployments" {
  git checkout --quiet main
  cat >apm.lock.yaml <<'LOCK'
dependencies:
- repo_url: example/tooling
  virtual_path: skills
  deployed_files: [vendor/skill]
- repo_url: _local/custom
  source: local
  local_path: ./custom
  deployed_files: [vendor/skill]
deployments:
- kind: project-relative
  value: vendor/skill
  active_owner: example/tooling/skills
LOCK
  git add -- apm.lock.yaml
  git commit --quiet -m 'test: active ownership'
  BASE_OID=$(git rev-parse HEAD)
  git checkout --quiet topic
  git rebase --quiet main
  change vendor/skill/lint.yaml 'ignore: [rule]'
  run inspect_policy
  [ "$status" -eq 0 ]
  [ "$(printf '%s' "$output" | jq '.findings|length')" -eq 0 ]
}

@test "removed lint commands in YAML lists require owner review" {
  git checkout --quiet main
  printf '%s\n' '- "mise run lint"' >lint.yaml
  git add -- lint.yaml
  git commit --quiet -m 'test: registered gate'
  BASE_OID=$(git rev-parse HEAD)
  git checkout --quiet topic
  git rebase --quiet main
  change lint.yaml '# gate removed'
  run inspect_policy
  [ "$status" -eq 0 ]
  [ "$(printf '%s' "$output" | jq '.findings|length')" -eq 1 ]
}

@test "empty policy patterns report the key and refusal reason" {
  local key config
  for key in golden_paths loosening_markers gate_config_paths owner_paths disposable_ignored; do
    config=$(policy_config | jq --arg key "$key" '.[$key] = ""')
    run uv run --quiet --script "$POLICY" inspect "$BASE_OID" "$HEAD_OID" <<<"$config"
    [ "$status" -eq 1 ]
    [[ "$output" == *"merge policy unreadable: Policy pattern $key must not be empty"* ]]
  done
}

@test "missing policy pattern reports its key" {
  run uv run --quiet --script "$POLICY" inspect "$BASE_OID" "$HEAD_OID" <<<'{}'
  [ "$status" -eq 1 ]
  [[ "$output" == *"Policy pattern golden_paths must not be empty"* ]]
}

@test "top-level policy refusals include the exception message" {
  run uv run --quiet --script "$POLICY" unknown
  [ "$status" -eq 1 ]
  [[ "$output" == *"merge policy unreadable: Unknown policy operation or arguments"* ]]
}

@test "devtools repository policy protects skill guards" {
  cp "$ROOT/.agents/agent-skills.conf" .agents/agent-skills.conf
  git add -- .agents/agent-skills.conf
  change packages/agents/source/skills/merge-pr/claude/scripts/guard-merge.sh '# guard'
  run bash "$AGENT/review.sh" 7
  [ "$status" -ne 0 ]
  [[ "$output" == *"packages/agents/source/skills/merge-pr/claude/scripts/guard-merge.sh"*"changes a golden file"* ]]
  [[ "$output" == *"needs-owner"* ]]
  [[ "$output" != *"merge policy unreadable"* ]]
}

@test "devtools golden and owner patterns cover only merge enforcement" {
  cp "$ROOT/.agents/agent-skills.conf" .agents/agent-skills.conf
  local key pattern path prefix harness skill script
  local protected=(
    .agents/agent-skills.conf
    .agents/review-instructions.md
    .github/rulesets/main.json
    .claude/settings.json
    packages/agents/source/lib/lib.sh
    packages/agents/source/lib/agent.sh
  )
  for harness in shared claude codex; do
    protected+=("packages/agents/source/skills/commit/$harness/scripts/guard-git.sh")
    for script in guard-merge.sh merge-agent.sh merge-policy.py; do
      protected+=("packages/agents/source/skills/merge-pr/$harness/scripts/$script")
    done
  done
  for prefix in packages/agents/{claude,codex}/.apm/skills .{claude,agents}/skills; do
    for skill in commit merge-pr agent-merge-pr; do
      for script in guard-git.sh lib.sh agent.sh; do
        protected+=("$prefix/$skill/scripts/$script")
      done
    done
    for skill in merge-pr agent-merge-pr; do
      for script in merge-agent.sh merge-policy.py; do
        protected+=("$prefix/$skill/scripts/$script")
      done
    done
  done
  local unprotected=(
    packages/agents/source/skills/pr/shared/scripts/create-pr.sh
    packages/agents/source/skills/merge-pr/shared/scripts/squash-merge.sh
    packages/agents/source/lib/other.sh
    .devtools/tasks/lint.toml
    .config/mise/conf.d/tools.toml
    .pre-commit-hooks.yaml
    mise.toml
    .github/workflows/ci.yml
    .claude/settings.json.backup
    .agents/agent-skills.conf.backup
    .agents/review-instructions.md.backup
  )
  for prefix in packages/agents/{claude,codex}/.apm/skills .{claude,agents}/skills; do
    unprotected+=("$prefix/pr/scripts/create-pr.sh" "$prefix/pr/scripts/merge-agent.sh"
      "$prefix/pr/scripts/merge-policy.py" "$prefix/merge-pr/SKILL.md"
      "$prefix/merge-pr/scripts/merge-agent.sh.backup" "$prefix/commit/scripts/guard-git.sh.backup")
  done
  for key in golden_paths owner_paths; do
    run bash -c 'source "$1/lib.sh"; dt_repo_conf "$2"' _ "$SCRIPTS" "$key"
    [ "$status" -eq 0 ]
    pattern=$output
    for path in "${protected[@]}"; do
      [[ "$path" =~ $pattern ]] || { echo "$key missed $path"; return 1; }
    done
    for path in "${unprotected[@]}"; do
      [[ ! "$path" =~ $pattern ]] || { echo "$key matched $path"; return 1; }
    done
  done
}
