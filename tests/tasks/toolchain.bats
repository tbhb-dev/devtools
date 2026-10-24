#!/usr/bin/env bats

setup() {
  TASK="${BATS_TEST_TMPDIR}/check.sh"
  awk '
    /^run = / { body = 1; next }
    body && /^\047\047\047$/ { exit }
    body { print }
  ' "${BATS_TEST_DIRNAME}/../../.devtools/tasks/toolchain.toml" > "$TASK"
  export MISE_CONFIG_DIR="${BATS_TEST_TMPDIR}/global"
  export MISE_DATA_DIR="${BATS_TEST_TMPDIR}/data"
  export MISE_CACHE_DIR="${BATS_TEST_TMPDIR}/cache"
  export MISE_TRUSTED_CONFIG_PATHS="$BATS_TEST_TMPDIR"
  mkdir -p "$MISE_CONFIG_DIR"
  printf '[tools]\nnode = "0.0.1"\n' > "$MISE_CONFIG_DIR/config.toml"
  PARENT="${BATS_TEST_TMPDIR}/parent"
  CHILD="$PARENT/.claude/worktrees/child"
  mkdir -p "$CHILD"
  git -C "$PARENT" init -q
  git -C "$PARENT" -c user.name=Test -c user.email=test@example.com commit -qm initial --allow-empty
  git -C "$PARENT" worktree add -q --detach "$CHILD"
  printf '[tools]\npython = "0.0.1"\n' > "$PARENT/mise.toml"
  printf '[tools]\n' > "$CHILD/mise.toml"
  cd "$CHILD"
}

@test "nested worktree excludes parent and global tools" {
  run bash "$TASK"
  [ "$status" -eq 0 ]
  [[ "$output" == *'toolchain matches'* ]]
}

@test "nested worktree still checks its own pins and lockfiles" {
  printf '[tools]\nnode = "0.0.2"\n' > mise.toml
  run bash "$TASK"
  [ "$status" -eq 1 ]
  [[ "$output" == *'toolchain is out of date'* ]]
}

@test "top-level checkout excludes global tools" {
  printf '[tools]\n' > "$PARENT/mise.toml"
  cd "$PARENT"
  run bash "$TASK"
  [ "$status" -eq 0 ]
  [[ "$output" == *'toolchain matches'* ]]
}
