#!/usr/bin/env bats

@test "bootstrap builds working-tree commands before git hooks" {
  root=$(git -C "$BATS_TEST_DIRNAME" rev-parse --show-toplevel)

  run mise -C "$root" task info bootstrap --json

  [ "$status" -eq 0 ]
  commands=$(jq -r '.run[]' <<<"$output")
  [ "$commands" = $'mise run install-toolchain\nmise run install-tools\nmise run build\nmise run devtools:prek-install\nmise run devtools:install-pre-push-hook' ]
}

worktree_setup() {
  root=$(git -C "$BATS_TEST_DIRNAME" rev-parse --show-toplevel)
  local line in_task=false in_body=false
  while IFS= read -r line; do
    if [ "$line" = '["devtools:worktree-setup"]' ]; then
      in_task=true
    elif "$in_task" && [ "$line" = "run = '''" ]; then
      in_body=true
    elif "$in_body"; then
      [ "$line" != "'''" ] || break
      printf '%s\n' "$line"
    fi
  done <"$root/.devtools/tasks/toolchain.toml" >"$BATS_TEST_TMPDIR/task.sh"

  bin="$BATS_TEST_TMPDIR/bin"
  mkdir -p "$bin"
  export CALLS="$BATS_TEST_TMPDIR/calls"
  cat >"$bin/mise" <<'SH'
#!/bin/bash
printf 'mise %s\n' "$*" >>"$CALLS"
printf '%s\n' "${TRUST:-config: trusted}"
SH
  cat >"$bin/vale" <<'SH'
#!/bin/bash
printf 'vale %s\n' "$*" >>"$CALLS"
echo 'Vale output'
exit "${VALE_STATUS:-0}"
SH
  cat >"$bin/uv" <<'SH'
#!/bin/bash
printf 'uv %s\n' "$*" >>"$CALLS"
echo 'uv output'
exit "${UV_STATUS:-0}"
SH
  chmod +x "$bin/mise" "$bin/vale" "$bin/uv"
  cd "$BATS_TEST_TMPDIR" || return
}

run_worktree_setup() {
  # Only the existing tools are available to the extracted task body (AC6).
  run env "$@" PATH="$bin" /bin/bash "$BATS_TEST_TMPDIR/task.sh"
  calls=$(<"$CALLS")
}

@test "AC1: missing Python manifest skips uv after syncing Vale" {
  worktree_setup
  touch .vale.ini
  run_worktree_setup
  [ "$status" -eq 0 ]
  [ "$calls" = $'mise trust --show\nvale sync' ]
  [ "$output" = $'Vale output\nworktree setup: skipping uv sync (no pyproject.toml)' ]
}

@test "AC2: both root manifests sync Vale before uv" {
  worktree_setup
  touch .vale.ini pyproject.toml
  run_worktree_setup
  [ "$status" -eq 0 ]
  [ "$calls" = $'mise trust --show\nvale sync\nuv sync' ]
  [ "$output" = $'Vale output\nuv output' ]
}

@test "AC3: missing Vale manifest skips Vale and still syncs uv" {
  worktree_setup
  touch pyproject.toml
  run_worktree_setup
  [ "$status" -eq 0 ]
  [ "$calls" = $'mise trust --show\nuv sync' ]
  [ "$output" = $'worktree setup: skipping vale sync (no .vale.ini)\nuv output' ]
}

@test "AC4: failing Vale preserves its status and prevents uv" {
  worktree_setup
  touch .vale.ini pyproject.toml
  run_worktree_setup VALE_STATUS=23
  [ "$status" -eq 23 ]
  [ "$calls" = $'mise trust --show\nvale sync' ]
  [ "$output" = 'Vale output' ]
}

@test "AC4: failing uv preserves its status after Vale succeeds" {
  worktree_setup
  touch .vale.ini pyproject.toml
  run_worktree_setup UV_STATUS=24
  [ "$status" -eq 24 ]
  [ "$calls" = $'mise trust --show\nvale sync\nuv sync' ]
  [ "$output" = $'Vale output\nuv output' ]
}

@test "AC4: Vale failure precedes the missing Python manifest skip" {
  worktree_setup
  touch .vale.ini
  run_worktree_setup VALE_STATUS=23
  [ "$status" -eq 23 ]
  [ "$calls" = $'mise trust --show\nvale sync' ]
  [ "$output" = 'Vale output' ]
}

@test "AC4: uv failure still fails after skipping absent Vale manifest" {
  worktree_setup
  touch pyproject.toml
  run_worktree_setup UV_STATUS=24
  [ "$status" -eq 24 ]
  [ "$calls" = $'mise trust --show\nuv sync' ]
  [ "$output" = $'worktree setup: skipping vale sync (no .vale.ini)\nuv output' ]
}

@test "AC5: untrusted config exits one with the trust hint before any sync" {
  worktree_setup
  touch .vale.ini pyproject.toml
  run_worktree_setup TRUST='config: untrusted'
  [ "$status" -eq 1 ]
  [ "$calls" = 'mise trust --show' ]
  [ "$output" = $'worktree setup: mise config is untrusted. Run:\n\n  mise trust' ]
}

@test "AC5: missing manifests do not bypass the trust check" {
  worktree_setup
  run_worktree_setup TRUST='config: untrusted'
  [ "$status" -eq 1 ]
  [ "$calls" = 'mise trust --show' ]
  [ "$output" = $'worktree setup: mise config is untrusted. Run:\n\n  mise trust' ]
}

@test "AC6: absent root manifests need no tools beyond mise and only add skip lines" {
  worktree_setup
  # Parent manifests must not enable worktree-local sync steps.
  touch .vale.ini pyproject.toml
  mkdir child
  cd child
  rm "$bin/vale" "$bin/uv"
  run_worktree_setup
  [ "$status" -eq 0 ]
  [ "$calls" = 'mise trust --show' ]
  [ "$output" = $'worktree setup: skipping vale sync (no .vale.ini)\nworktree setup: skipping uv sync (no pyproject.toml)' ]
}
