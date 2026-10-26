#!/usr/bin/env bats
# Compatibility declarations apply only to consumer-facing branch diffs.

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  pr_setup
  printf '\n## Risk\n' >>.github/pull_request_template.md
  printf '\n## Risk\n\nRevert the change.\n' >>PR_AGENTDESC.md
}

consumer_change() {
  mkdir -p "$(dirname "$1")"
  commit_file "$1" 'feat: update consumer configuration'
}

missing_declaration() {
  consumer_change "$1"
  run bash "$SCRIPTS/validate-description.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *"compatibility-missing"* ]]
  [[ "$output" == *"Compatibility: none"* ]]
  [[ "$output" == *"Compatibility: deprecates <key> since <version>"* ]]
}

@test "AC3 .devtools changes require compatibility (#661)" {
  missing_declaration .devtools/example.txt
}

@test "AC3 tool pin changes require compatibility (#661)" {
  missing_declaration .config/mise/conf.d/example.toml
}

@test "AC3 agent changes require compatibility (#661)" {
  missing_declaration packages/agents/example.txt
}

@test "AC3 vendir tasks require compatibility (#661)" {
  missing_declaration vendir.toml
}

@test "AC3 pin tasks require compatibility (#661)" {
  missing_declaration pins.toml
}

@test "AC3 apm tasks require compatibility (#661)" {
  missing_declaration apm.toml
}

@test "AC3 nested task files require compatibility (#661)" {
  missing_declaration tasks/pins.toml
}

@test "AC3 accepts both compatibility declarations in Risk (#661)" {
  consumer_change packages/agents/example.txt
  cp PR_AGENTDESC.md original.md
  for declaration in 'none' 'deprecates old_key since v0.23.0'; do
    cp original.md PR_AGENTDESC.md
    printf '\nCompatibility: %s\n' "$declaration" >>PR_AGENTDESC.md
    run bash "$SCRIPTS/validate-description.sh"
    [ "$status" -eq 0 ]
  done
}

@test "AC3 ignores declarations outside Risk and malformed lines (#661)" {
  consumer_change .devtools/example.txt
  cp PR_AGENTDESC.md original.md
  for declaration in 'none extra' 'deprecates key since' \
    'deprecates key v0.23.0'; do
    cp original.md PR_AGENTDESC.md
    printf '\nCompatibility: %s\n' "$declaration" >>PR_AGENTDESC.md
    run bash "$SCRIPTS/validate-description.sh"
    [ "$status" -eq 1 ]
    [[ "$output" == *"compatibility-missing"* ]]
  done
  cp original.md PR_AGENTDESC.md
  printf '\n## Other\n\nCompatibility: none\n' >>PR_AGENTDESC.md
  run bash "$SCRIPTS/validate-description.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *"compatibility-missing"* ]]
}

@test "AC3 other changes need no compatibility line (#661)" {
  run bash "$SCRIPTS/validate-description.sh"
  [ "$status" -eq 0 ]
}

@test "AC2 scaffold carries the compatibility instruction (#661)" {
  cp "$ROOT/.github/pull_request_template.md" \
    .github/pull_request_template.md
  rm PR_AGENTDESC.md
  run bash "$SCRIPTS/scaffold-description.sh"
  [ "$status" -eq 0 ]
  grep -q '^## Risk$' PR_AGENTDESC.md
  grep -q '^Compatibility: none$' PR_AGENTDESC.md
  grep -q 'Compatibility: deprecates <key> since <version>' \
    PR_AGENTDESC.md
}

@test "AC3 moving a consumer file out still needs a declaration (#661)" {
  git checkout --quiet main
  mkdir -p .devtools
  git add -- .github/pull_request_template.md
  commit_file .devtools/example.txt 'chore: add consumer configuration'
  git push --quiet origin main
  git checkout --quiet topic
  git rebase --quiet main
  git mv .devtools/example.txt example.txt
  git commit --quiet -m 'feat: move consumer configuration'
  run bash "$SCRIPTS/validate-description.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *"compatibility-missing"* ]]
}
