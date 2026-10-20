#!/usr/bin/env bats

setup() {
  ROOT="$(cd "${BATS_TEST_DIRNAME}/../../.." && pwd)"
  TEST_TEMP_DIR="$(mktemp -d)"
  export TEST_TEMP_DIR
  export PATH="$TEST_TEMP_DIR/bin:$PATH"

  mkdir -p "$TEST_TEMP_DIR/bin" "$TEST_TEMP_DIR/repo"
  git -C "$TEST_TEMP_DIR/repo" init -q -b main
  git -C "$TEST_TEMP_DIR/repo" config user.name Test
  git -C "$TEST_TEMP_DIR/repo" config user.email test@example.com
  git -C "$TEST_TEMP_DIR/repo" -c commit.gpgSign=false commit --allow-empty -qm initial

  cat >"$TEST_TEMP_DIR/bin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

if [ "$1 $2" = "auth status" ]; then
  exit 0
fi

if [ "$1 $2" = "pr view" ]; then
  printf '%s\n' '{"title":"fix: current title","body":"## Summary\n\nPublished body.","baseRefName":"main","isDraft":false,"labels":[{"name":"bug"}],"assignees":[],"milestone":null}'
  exit 0
fi

exit 2
EOF
  chmod +x "$TEST_TEMP_DIR/bin/gh"
  cd "$TEST_TEMP_DIR/repo"
}

teardown() {
  cd /
  rm -rf "$TEST_TEMP_DIR"
}

@test "a stale existing draft returns an explicit hold without changing it (#296)" {
  printf 'draft in progress\n' >PR_AGENTDESC.md

  run bash "$ROOT/packages/agents/source/skills/fix-pr/shared/scripts/populate-description.sh" 294

  [ "$status" -eq 75 ]
  [[ "$output" == *"status: hold"* ]]
  [ "$(cat PR_AGENTDESC.md)" = "draft in progress" ]
}

@test "a current existing draft is retained and classified (#296)" {
  run bash "$ROOT/packages/agents/source/skills/fix-pr/shared/scripts/populate-description.sh" 294
  [ "$status" -eq 0 ]
  [[ "$output" == *"status: refreshed"* ]]

  run bash "$ROOT/packages/agents/source/skills/fix-pr/shared/scripts/populate-description.sh" 294
  [ "$status" -eq 0 ]
  [[ "$output" == *"status: current"* ]]
}

@test "force replaces a stale draft explicitly (#296)" {
  printf 'stale draft\n' >PR_AGENTDESC.md

  run bash "$ROOT/packages/agents/source/skills/fix-pr/shared/scripts/populate-description.sh" --force 294

  [ "$status" -eq 0 ]
  [[ "$output" == *"status: refreshed"* ]]
  grep -Fq '# fix: current title' PR_AGENTDESC.md
  ! grep -Fq 'stale draft' PR_AGENTDESC.md
}
