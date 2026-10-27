#!/usr/bin/env bats

setup_file() {
  export ROOT
  ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
  mkdir -p "$BATS_FILE_TMPDIR/go-bin"
  (cd "$ROOT" && go build -o "$BATS_FILE_TMPDIR/go-bin/agentrepo" ./cmd/agentrepo)
  export GO_BIN="$BATS_FILE_TMPDIR/go-bin"
}

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  export REPO="$BATS_TEST_TMPDIR/repo" BIN="$BATS_TEST_TMPDIR/bin"
  mkdir -p "$REPO" "$BIN"
  git init --quiet --initial-branch=main "$REPO"
  cd "$REPO"
  export REQUESTS="$BATS_TEST_TMPDIR/requests"
  cat >"$BIN/uv" <<'STUB'
#!/usr/bin/env python3
import os, sys
text = sys.stdin.read()
with open(os.environ['REQUESTS'], 'a') as f:
    f.write('uv ' + sys.argv[-1] + '\n')
if 'FAIL' in text:
    print(10)
    sys.exit(23)
print(len(text.encode()))
STUB
  cat >"$BIN/ant" <<'STUB'
#!/usr/bin/env python3
import json, os, sys
request = sys.stdin.read()
with open(os.environ['REQUESTS'], 'a') as f:
    f.write(request.split("\n")[0] + '\n')
text = '\n'.join(line[6:] for line in request.split("\n")[4:-1])
print(json.dumps({'input_tokens': len(text.encode()) + 7}))
if 'FAIL' in text:
    sys.exit(23)
STUB
  chmod +x "$BIN/uv" "$BIN/ant"
  export PATH="$BIN:$GO_BIN:$PATH"
  CODEX=packages/agents/codex/.apm/skills/example
  CLAUDE=packages/agents/claude/.apm/skills/example
  DEPLOYED=.agents/skills/example
  for skill in "$CODEX" "$CLAUDE" "$DEPLOYED"; do
    mkdir -p "$skill"
    printf '%s\n' '---' 'name: example' 'description: >-' '  Folded café' '  description.' '---' '' 'Body.' >"$skill/SKILL.md"
    printf 'tracked text\n' >"$skill/tracked.txt"
    git add -- "$skill/tracked.txt"
    printf 'untracked text\n' >"$skill/é.txt"
    touch "$skill/empty"
    printf 'a\0b' >"$skill/binary"
    printf 'ignored\n' >"$skill/ignored"
  done
  printf 'ignored\n' >.gitignore
}

compare_frozen() {
  bash "$ROOT/tools/legacy/skill-tokens.sh" "$@" >"$BATS_TEST_TMPDIR/old-out" 2>"$BATS_TEST_TMPDIR/old-err"
  for skill in "$CODEX" "$CLAUDE" "$DEPLOYED"; do
    if [ -f "$skill/tokens.json" ]; then cp "$skill/tokens.json" "$BATS_TEST_TMPDIR/$(echo "$skill" | tr / _).json"; fi
  done
  cp "$REQUESTS" "$BATS_TEST_TMPDIR/old-requests"
  : >"$REQUESTS"
  bash "$ROOT/tools/skill-tokens.sh" "$@" >"$BATS_TEST_TMPDIR/new-out" 2>"$BATS_TEST_TMPDIR/new-err"
  diff -u "$BATS_TEST_TMPDIR/old-out" "$BATS_TEST_TMPDIR/new-out"
  diff -u "$BATS_TEST_TMPDIR/old-err" "$BATS_TEST_TMPDIR/new-err"
  diff -u "$BATS_TEST_TMPDIR/old-requests" "$REQUESTS"
  for skill in "$CODEX" "$CLAUDE" "$DEPLOYED"; do
    if [ -f "$skill/tokens.json" ]; then diff -u "$BATS_TEST_TMPDIR/$(echo "$skill" | tr / _).json" "$skill/tokens.json"; fi
  done
}

@test "default discovery equals frozen records and transcript" {
  touch packages/agents/codex/.apm/skills/plain-file
  compare_frozen
}

@test "hidden wildcard components are excluded from default discovery" {
  for skill in packages/agents/codex/.apm/skills/.hidden packages/agents/.hidden/.apm/skills/example; do
    mkdir -p "$skill"
    printf 'FAIL' >"$skill/SKILL.md"
  done
  compare_frozen
  [ ! -f packages/agents/codex/.apm/skills/.hidden/tokens.json ]
  [ ! -f packages/agents/.hidden/.apm/skills/example/tokens.json ]
}

@test "Unicode skill name equals frozen records and exact transcript" {
  mv "$CODEX" "${CODEX%/*}/café"
  CODEX="${CODEX%/*}/café"
  compare_frozen "$CODEX"
}

@test "explicit absolute deployed and absent paths equal frozen output" {
  compare_frozen "$REPO/$CODEX/" "$DEPLOYED/" missing
}

@test "model and encoding precedence and advisory budgets equal frozen output" {
  export MODEL=generic CLAUDE_MODEL=claude-explicit CODEX_MODEL=codex-explicit CODEX_ENCODING=encoding-explicit BODY_BUDGET=6
  compare_frozen "$CLAUDE" "$CODEX" "$CLAUDE"
}

@test "MODEL fallback and empty UTF-8 manifests equal frozen output" {
  export MODEL=generic BODY_BUDGET=1
  unset CLAUDE_MODEL
  printf '' >"$CLAUDE/SKILL.md"
  printf 'café\n' >"$CODEX/SKILL.md"
  compare_frozen "$CLAUDE" "$CODEX"
}

@test "supported failed count does not fall back or repeat a request" {
  printf 'FAIL' >"$CODEX/SKILL.md"
  run bash "$ROOT/tools/skill-tokens.sh" "$CODEX"
  [ "$status" -ne 0 ]
  [ ! -f "$CODEX/tokens.json" ]
  [ "$(wc -l <"$REQUESTS")" -eq 1 ]
}

@test "Claude partial output failure preserves completed earlier record" {
  printf 'FAIL' >"$CLAUDE/SKILL.md"
  run bash "$ROOT/tools/skill-tokens.sh" "$CODEX" "$CLAUDE"
  [ "$status" -ne 0 ]
  [ -f "$CODEX/tokens.json" ]
  [ ! -f "$CLAUDE/tokens.json" ]
  [ "$(grep -c '^model:' "$REQUESTS")" -eq 2 ]
}

@test "capability probe makes no request or write and failed probe never falls back" {
  run agentrepo --help
  [ "$status" -eq 0 ]
  [ ! -f "$REQUESTS" ]
  [ ! -f "$CODEX/tokens.json" ]
  printf '#!/bin/sh\nexit 23\n' >"$BIN/agentrepo"
  chmod +x "$BIN/agentrepo"
  run bash "$ROOT/tools/skill-tokens.sh" "$CODEX"
  [ "$status" -eq 23 ]
  [ ! -f "$REQUESTS" ]
}

@test "older binary falls back with unchanged environment and arguments" {
  printf '#!/bin/sh\necho "Usage: agentrepo checks"\n' >"$BIN/agentrepo"
  chmod +x "$BIN/agentrepo"
  export CODEX_MODEL=older-compatible CODEX_ENCODING=older-encoding
  compare_frozen "$CODEX/" missing
}

@test "absent binary preserves N-2 wrapper invocation" {
  export PATH="$BIN:/usr/bin:/bin"
  compare_frozen "$CODEX/" missing
}
