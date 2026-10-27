#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
  REPO="${BATS_TEST_TMPDIR}/repo"
  BIN="${BATS_TEST_TMPDIR}/bin"
  SKILL="${REPO}/packages/agents/codex/.apm/skills/codex-test"
  mkdir -p "$BIN" "$SKILL"
  git init --quiet --initial-branch=main "$REPO"
  cd "$REPO"
  printf '%s\n' '#!/bin/sh' 'wc -c | tr -d " "' >"${BIN}/uv"
  printf '%s\n' '#!/bin/sh' 'touch "${ANT_CALLED}"' 'exit 99' >"${BIN}/ant"
  chmod +x "${BIN}/uv" "${BIN}/ant"
  cat >"${SKILL}/SKILL.md" <<'EOF'
---
name: codex-test
description: >-
  Count this Codex skill locally.
---

# Test

One paragraph.
EOF
}

@test "Codex skills use local tiktoken without Anthropic authentication" {
  run env PATH="${BIN}:${PATH}" ANT_CALLED="${BATS_TEST_TMPDIR}/ant-called" MODEL=claude-override \
    bash "${ROOT}/tools/legacy/skill-tokens.sh" "$SKILL"

  [ "$status" -eq 0 ] || { echo "$output"; false; }
  [ ! -e "${BATS_TEST_TMPDIR}/ant-called" ]
  [ "$(jq -r .model "${SKILL}/tokens.json")" = gpt-5.6-sol ]
  [ "$(jq -r .tokenizer "${SKILL}/tokens.json")" = o200k_base ]
}

@test "relative script invocation ignores an exported cd search path" {
  mkdir -p "$REPO/tools/legacy"
  cp "$ROOT/tools/skill-tokens.sh" "$REPO/tools/skill-tokens.sh"
  cp "$ROOT/tools/legacy/skill-tokens.sh" "$REPO/tools/legacy/skill-tokens.sh"
  printf '%s\n' '#!/bin/sh' 'echo "  other  not the token command"' >"${BIN}/agentrepo"
  chmod +x "${BIN}/agentrepo"
  cd "$REPO"
  run env "CD""PATH=$REPO" PATH="${BIN}:${PATH}" bash tools/skill-tokens.sh "$SKILL"

  [ "$status" -eq 0 ] || { echo "$output"; false; }
  [ -e "${SKILL}/tokens.json" ]
}

@test "token counter cannot write a record after a bundled tokenizer failure" {
  echo 'bundled content' >"$SKILL/extra.txt"
  cat >"$BIN/uv" <<'STUB'
#!/bin/bash
text=$(cat)
if [[ $text == *'bundled content'* ]]; then echo 10; exit 23; fi
printf '%s' "$text" | wc -c
STUB
  run env PATH="$BIN:$PATH" bash "$ROOT/tools/legacy/skill-tokens.sh" "$SKILL"
  [ "$status" -ne 0 ]
  [ ! -e "$SKILL/tokens.json" ]
}

@test "token counter rejects failed bundled file discovery" {
  real_git=$(command -v git)
  cat >"$BIN/git" <<STUB
#!/bin/bash
if [[ \$* == *ls-files* ]]; then exit 23; fi
exec "$real_git" "\$@"
STUB
  chmod +x "$BIN/git"
  run env PATH="$BIN:$PATH" bash "$ROOT/tools/legacy/skill-tokens.sh" "$SKILL"
  [ "$status" -eq 23 ]
  [ ! -e "$SKILL/tokens.json" ]
}

@test "token counter rejects failed manifest byte measurements" {
  printf '#!/bin/sh\necho 10\n' >"$BIN/uv"
  printf '#!/bin/sh\necho 10\nexit 23\n' >"$BIN/wc"
  chmod +x "$BIN/wc"
  run env PATH="$BIN:$PATH" bash "$ROOT/tools/legacy/skill-tokens.sh" "$SKILL"
  [ "$status" -eq 23 ]
  [ ! -e "$SKILL/tokens.json" ]
}

@test "Claude token failures propagate through the envelope subtraction" {
  sed -n '/^net() {/,/^}/p' "$ROOT/tools/legacy/skill-tokens.sh" >net.sh
  cat >>net.sh <<'SH'
TOKEN_BACKEND=claude
claude_baseline=1
count_claude() { echo 10; return 23; }
net /dev/null
SH
  run bash net.sh
  [ "$status" -eq 23 ]
}

@test "token counter propagates a failed bundled total calculation" {
  printf '#!/bin/sh\necho 0\nexit 23\n' >"$BIN/awk"
  chmod +x "$BIN/awk"
  run env PATH="$BIN:$PATH" bash "$ROOT/tools/legacy/skill-tokens.sh" "$SKILL"
  [ "$status" -eq 23 ]
}
