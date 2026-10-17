#!/usr/bin/env bats
# tools/generate-skills.sh against a scratch repository holding a small
# source tree, so the cases can break the output on purpose.

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  GEN="${BATS_TEST_DIRNAME}/../../tools/generate-skills.sh"
  REPO="${BATS_TEST_TMPDIR}/repo"
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
  git init --quiet --initial-branch=main "$REPO"
  cd "$REPO"
  src=packages/agents/source/skills/demo
  mkdir -p "$src/parts" "$src/shared/scripts" "$src/claude/scripts" packages/agents/source/lib
  printf 'lib\n' >packages/agents/source/lib/lib.sh
  printf -- '---\nname: demo\n---\n\n# Claude\n\n@include parts/body.md\n' >"$src/SKILL.claude.md"
  printf -- '---\nname: demo\n---\n\n# Codex\n\n@include parts/body.md\n' >"$src/SKILL.codex.md"
  printf 'Shared body.\n' >"$src/parts/body.md"
  printf '#!/usr/bin/env bash\n. "$(dirname "$0")/lib.sh"\n' >"$src/shared/scripts/run.sh"
  chmod +x "$src/shared/scripts/run.sh"
  printf '#!/usr/bin/env bash\n' >"$src/claude/scripts/guard.sh"
  chmod +x "$src/claude/scripts/guard.sh"
  CLAUDE_OUT=packages/agents/claude/.apm/skills/demo
  CODEX_OUT=packages/agents/codex/.apm/skills/demo
}

@test "generate renders includes and copies shared, harness-only, and lib files" {
  run bash "$GEN"
  [ "$status" -eq 0 ]

  grep -q '^# Claude$' "$CLAUDE_OUT/SKILL.md"
  grep -q '^Shared body\.$' "$CLAUDE_OUT/SKILL.md"
  [ "$(grep -c '@include' "$CLAUDE_OUT/SKILL.md")" = 0 ]
  grep -q '^# Codex$' "$CODEX_OUT/SKILL.md"
  [ -x "$CLAUDE_OUT/scripts/run.sh" ]
  [ -x "$CLAUDE_OUT/scripts/guard.sh" ]
  [ ! -e "$CODEX_OUT/scripts/guard.sh" ]
  [ "$(cat "$CODEX_OUT/scripts/lib.sh")" = lib ]
  [ ! -e "$CODEX_OUT/parts" ]
}

@test "generate leaves out the library where no script mentions it" {
  printf '#!/usr/bin/env bash\n' >"$src/shared/scripts/run.sh"
  bash "$GEN"
  [ ! -e "$CLAUDE_OUT/scripts/lib.sh" ]
}

@test "generate keeps tokens.json and removes stale files" {
  mkdir -p "$CLAUDE_OUT"
  printf '{}\n' >"$CLAUDE_OUT/tokens.json"
  printf 'stale\n' >"$CLAUDE_OUT/stale.md"

  bash "$GEN"

  [ "$(cat "$CLAUDE_OUT/tokens.json")" = '{}' ]
  [ ! -e "$CLAUDE_OUT/stale.md" ]
}

@test "check passes on fresh output and ignores tokens.json" {
  bash "$GEN"
  printf '{"x":1}\n' >"$CLAUDE_OUT/tokens.json"

  run bash "$GEN" --check

  [ "$status" -eq 0 ]
  [ -z "$output" ]
}

@test "check reports edited bytes, an extra file, a lost exec bit, and a missing output" {
  bash "$GEN"
  printf 'hand edit\n' >>"$CLAUDE_OUT/SKILL.md"
  printf 'extra\n' >"$CLAUDE_OUT/extra.md"
  chmod -x "$CODEX_OUT/scripts/run.sh"

  run bash "$GEN" --check

  [ "$status" -eq 1 ]
  [[ "$output" == *"$CLAUDE_OUT/SKILL.md:1 [error] generated-drift"* ]]
  [[ "$output" == *"$CLAUDE_OUT/extra.md:1 [error] generated-drift"* ]]
  [[ "$output" == *"$CODEX_OUT/scripts/run.sh:1 [error] generated-drift"* ]]

  rm -rf "$CODEX_OUT"
  run bash "$GEN" --check
  [[ "$output" == *"$CODEX_OUT:1 [error] not-generated"* ]]
}

@test "a package skill without a source directory is left alone" {
  mkdir -p packages/agents/codex/.apm/skills/manual
  printf 'hand maintained\n' >packages/agents/codex/.apm/skills/manual/SKILL.md

  bash "$GEN"
  run bash "$GEN" --check

  [ "$status" -eq 0 ]
  [ "$(cat packages/agents/codex/.apm/skills/manual/SKILL.md)" = 'hand maintained' ]
}

@test "a harness without a template gets no output" {
  rm "$src/SKILL.codex.md"
  bash "$GEN"
  [ ! -e "$CODEX_OUT" ]
}

@test "a missing include fails the run" {
  printf '@include parts/missing.md\n' >>"$src/SKILL.claude.md"
  run bash "$GEN"
  [ "$status" -ne 0 ]
  [[ "$output" == *"includes parts/missing.md, which does not exist"* ]]
}

@test "a path in both shared and a harness tree fails the run" {
  printf '#!/usr/bin/env bash\n' >"$src/claude/scripts/run.sh"
  run bash "$GEN"
  [ "$status" -ne 0 ]
  [[ "$output" == *"scripts/run.sh exists in both shared/ and claude/"* ]]
}

@test "check rejects hash failures even when both listings print identical hashes" {
  bash "$GEN"
  mkdir -p bin
  real_git=$(command -v git)
  cat >bin/git <<EOF_STUB
#!/usr/bin/env bash
if [[ \$1 == hash-object ]]; then
  echo deadbeef
  exit 23
fi
exec "$real_git" "\$@"
EOF_STUB
  chmod +x bin/git
  run env PATH="$PWD/bin:$PATH" bash "$GEN" --check
  [ "$status" -eq 23 ]
}

@test "generation rejects failed source discovery instead of copying an incomplete tree" {
  mkdir -p bin
  real_find=$(command -v find)
  cat >bin/find <<EOF_STUB
#!/usr/bin/env bash
if [[ \$1 == */shared ]]; then exit 23; fi
exec "$real_find" "\$@"
EOF_STUB
  chmod +x bin/find
  run env PATH="$PWD/bin:$PATH" bash "$GEN"
  [ "$status" -eq 23 ]
}

@test "generation rejects a failed output parent lookup" {
  mkdir -p bin
  printf '#!/bin/sh\necho scripts\nexit 23\n' >bin/dirname
  chmod +x bin/dirname
  run env PATH="$PWD/bin:$PATH" bash "$GEN"
  [ "$status" -eq 23 ]
}

@test "check rejects diff errors instead of reporting no drift" {
  bash "$GEN"
  mkdir -p bin
  printf '#!/bin/sh\nexit 23\n' >bin/diff
  chmod +x bin/diff
  run env PATH="$PWD/bin:$PATH" bash "$GEN" --check
  [ "$status" -eq 23 ]
}
