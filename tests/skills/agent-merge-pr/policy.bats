#!/usr/bin/env bats
# cspell:words ignorecase noqa
setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup
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

@test "APM remote deployed paths are skipped while local overrides are scanned" {
  git checkout --quiet main
  cat >apm.lock.yaml <<'LOCK'
dependencies:
- repo_url: example/tooling
  deployed_files: [vendor/skills]
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
