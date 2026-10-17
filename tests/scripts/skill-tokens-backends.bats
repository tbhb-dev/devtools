#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  ROOT="${BATS_TEST_DIRNAME}/../.."
  REPO="${BATS_TEST_TMPDIR}/repo"
  BIN="${BATS_TEST_TMPDIR}/bin"
  SKILL="${REPO}/packages/agents/codex/.apm/skills/codex-test"
  mkdir -p "$BIN" "$SKILL"
  git init --quiet --initial-branch=main "$REPO"
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
    bash "${ROOT}/tools/skill-tokens.sh" "$SKILL"

  [ "$status" -eq 0 ]
  [ ! -e "${BATS_TEST_TMPDIR}/ant-called" ]
  [ "$(jq -r .model "${SKILL}/tokens.json")" = gpt-5.6-sol ]
  [ "$(jq -r .tokenizer "${SKILL}/tokens.json")" = o200k_base ]
}
