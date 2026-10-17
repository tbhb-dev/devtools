#!/usr/bin/env bats

setup() {
  source "$BATS_TEST_DIRNAME/../../../test_helper.bash"
  isolate_git
  stub_gates
  LIB="$BATS_TEST_DIRNAME/../../../../packages/agents/claude/.apm/skills/commit/scripts/lib.sh"
  git init --quiet "$BATS_TEST_TMPDIR/repo"
  cd "$BATS_TEST_TMPDIR/repo"
  git config user.name Tester
  git config user.email tester@example.com
  printf 'draft\n' >.gitignore
  printf 'initial\n' >tracked
  git add -- .gitignore tracked
  git commit --quiet -m initial
  printf 'description\n' >draft
}

gate() {
  bash -c '. "$1"; dt_gate "$2" "$3" "$4" mise run "$2"' _ "$LIB" "${1:-test}" "${2:-}" "${3:-false}"
}

@test "clean passes reuse and forced runs override (#161)" {
  gate
  run gate
  [ "$status" -eq 0 ]
  [[ "$output" == *"(reused)"* ]]
  gate test '' true
  [ "$(cat "$GATE_LOG")" = $'test\ntest' ]
}

@test "staged, unstaged, and untracked files prevent reuse (#161)" {
  gate
  printf 'dirty\n' >>tracked
  gate
  git add -- tracked
  gate
  git reset --quiet --hard HEAD
  printf 'untracked\n' >extra
  gate
  [ "$(cat "$GATE_LOG")" = $'test\ntest\ntest\ntest' ]
}

@test "committed tree changes invalidate passes (#161)" {
  gate
  printf 'new\n' >>tracked
  git add -- tracked
  git commit --quiet -m changed
  gate
  [ "$(cat "$GATE_LOG")" = $'test\ntest' ]
}

@test "description hash invalidates its gate without invalidating test (#161)" {
  gate lint-pr-description draft
  gate
  printf 'edited\n' >>draft
  gate lint-pr-description draft
  gate
  [ "$(cat "$GATE_LOG")" = $'lint-pr-description\ntest\nlint-pr-description' ]
}

@test "ignored gate definitions and hooks invalidate passes (#161)" {
  printf 'mise.toml\n.config/\n.devtools/\n' >>.gitignore
  git add -- .gitignore
  git commit --quiet -m ignores
  for path in mise.toml .config/mise/conf.d/tools.toml .devtools/tasks/test .git/hooks/pre-commit; do
    gate
    parent=$(dirname "$path")
    mkdir -p "$parent"
    printf 'changed\n' >"$path"
    run gate
    [ "$status" -eq 0 ]
    [[ "$output" != *"(reused)"* ]]
  done
}

@test "mise and tool versions invalidate passes (#161)" {
  gate
  export GATE_VERSION=mise-new
  gate
  export TOOL_VERSION='{"tool":"new"}'
  gate
  [ "$(cat "$GATE_LOG")" = $'test\ntest\ntest' ]
}

@test "gate name and exact command belong to the key (#161)" {
  gate
  bash -c '. "$1"; dt_gate test "" false mise run other' _ "$LIB"
  gate lint
  [ "$(cat "$GATE_LOG")" = $'test\nother\nlint' ]
}

@test "failure invalidates even an earlier pass (#161)" {
  gate
  export FAIL_GATE=test
  run gate test '' true
  [ "$status" -eq 1 ]
  run gate
  [ "$status" -eq 1 ]
  unset FAIL_GATE
  gate
  [ "$(cat "$GATE_LOG")" = $'test\ntest\ntest\ntest' ]
}

@test "expired passes rerun (#161)" {
  gate
  python3 - <<'PY'
import json
from pathlib import Path
for path in Path('.git/agent-skills/gates').glob('*.json'):
    record = json.loads(path.read_text())
    record['passed'] -= 3601
    path.write_text(json.dumps(record))
PY
  gate
  [ "$(cat "$GATE_LOG")" = $'test\ntest' ]
}

@test "a gate that dirties the tree cannot record a pass (#161)" {
  bash -c '. "$1"; dt_gate test "" false touch extra' _ "$LIB"
  rm extra
  gate
  [ "$(cat "$GATE_LOG")" = test ]
}
