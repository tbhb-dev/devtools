#!/usr/bin/env bats
# tools/generate-skills.sh against a scratch repository holding a small
# source tree, so the cases can break the output on purpose.

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  GEN="${BATS_TEST_DIRNAME}/../../tools/generate-skills.sh"
  FROZEN="${BATS_TEST_DIRNAME}/../../tools/compat/generate-skills.sh"
  REPO="${BATS_TEST_TMPDIR}/repo"
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
  git init --quiet --initial-branch=main "$REPO"
  cd "$REPO"
  src=packages/agents/source/skills/demo
  mkdir -p "$src/parts" "$src/shared/scripts" "$src/claude/scripts" packages/agents/source/lib
  printf 'lib\n' >packages/agents/source/lib/lib.sh
  printf 'gate helper\n' >packages/agents/source/lib/gates.py
  printf 'agent helper\n' >packages/agents/source/lib/agent.sh
  printf 'signing helper\n' >packages/agents/source/lib/signing.sh
  printf 'worktree helper\n' >packages/agents/source/lib/worktree-ready.sh
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
  [ "$(cat "$CODEX_OUT/scripts/gates.py")" = 'gate helper' ]
  [ "$(cat "$CODEX_OUT/scripts/agent.sh")" = 'agent helper' ]
  [ "$(cat "$CODEX_OUT/scripts/signing.sh")" = 'signing helper' ]
  [ ! -e "$CODEX_OUT/parts" ]
}

@test "generate leaves out the library where no script mentions it" {
  printf '#!/usr/bin/env bash\n' >"$src/shared/scripts/run.sh"
  bash "$GEN"
  [ ! -e "$CLAUDE_OUT/scripts/lib.sh" ]
}

@test "generate copies the worktree helper where a script mentions it" {
  printf '#!/usr/bin/env bash\nbash "$(dirname "$0")/worktree-ready.sh"\n' >"$src/shared/scripts/run.sh"

  bash "$GEN"

  [ "$(cat "$CLAUDE_OUT/scripts/worktree-ready.sh")" = 'worktree helper' ]
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
  bash "$FROZEN"
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
  run env PATH="$PWD/bin:$PATH" bash "$FROZEN" --check
  [ "$status" -eq 23 ]
}

@test "generation rejects failed source discovery instead of copying an incomplete tree" {
  mkdir -p bin
  real_find=$(command -v find)
  cat >bin/find <<EOF_STUB
#!/usr/bin/env bash
if [[ \$1 == */shared ]]; then printf '%s\\n' "\$1/scripts/run.sh"; exit 23; fi
exec "$real_find" "\$@"
EOF_STUB
  chmod +x bin/find
  run env PATH="$PWD/bin:$PATH" bash "$FROZEN"
  [ "$status" -eq 23 ]
}

@test "generation rejects a failed output parent lookup" {
  mkdir -p bin
  printf '#!/bin/sh\necho scripts\nexit 23\n' >bin/dirname
  chmod +x bin/dirname
  run env PATH="$PWD/bin:$PATH" bash "$FROZEN"
  [ "$status" -eq 23 ]
}

@test "check rejects diff errors instead of reporting no drift" {
  bash "$FROZEN"
  mkdir -p bin
  printf '#!/bin/sh\nexit 23\n' >bin/diff
  chmod +x bin/diff
  run env PATH="$PWD/bin:$PATH" bash "$FROZEN" --check
  [ "$status" -eq 23 ]
}

@test "Go and frozen generation agree on trees, modes and check findings" {
  printf 'unterminated' >>"$src/SKILL.claude.md"
  mkdir -p "$CLAUDE_OUT"
  printf 'tokens\n' >"$CLAUDE_OUT/tokens.json"
  printf 'stale\n' >"$CLAUDE_OUT/stale"
  run bash "$FROZEN"
  [ "$status" -eq 0 ]
  frozen_output=$output
  cp -pR packages/agents/claude expected-claude
  cp -pR packages/agents/codex expected-codex
  run agentrepo generate-skills
  [ "$status" -eq 0 ]
  [ "$output" = "$frozen_output" ]
  diff -r expected-claude packages/agents/claude
  diff -r expected-codex packages/agents/codex
  [ -x "$CLAUDE_OUT/scripts/run.sh" ]
  [ ! -x "$CLAUDE_OUT/SKILL.md" ]
  printf 'edit\n' >>"$CLAUDE_OUT/SKILL.md"
  printf extra >"$CLAUDE_OUT/extra"
  chmod -x "$CODEX_OUT/scripts/run.sh"
  run bash "$FROZEN" --check
  expected=$output
  expected_status=$status
  git add -- packages
  git status --porcelain=v1 >"$BATS_TEST_TMPDIR/before-status"
  git ls-files --stage >"$BATS_TEST_TMPDIR/before-index"
  run agentrepo generate-skills --check
  [ "$status" -eq "$expected_status" ]
  [ "$output" = "$expected" ]
  git status --porcelain=v1 >"$BATS_TEST_TMPDIR/after-status"
  git ls-files --stage >"$BATS_TEST_TMPDIR/after-index"
  diff "$BATS_TEST_TMPDIR/before-status" "$BATS_TEST_TMPDIR/after-status"
  diff "$BATS_TEST_TMPDIR/before-index" "$BATS_TEST_TMPDIR/after-index"
  grep -q edit "$CLAUDE_OUT/SKILL.md"
}

@test "wrapper falls back only for absent capability and never repeats Go failure" {
  mkdir -p adapter/compat bin
  cp "$GEN" adapter/generate-skills.sh
  cat >adapter/compat/generate-skills.sh <<'STUB'
#!/bin/sh
printf 'fallback:%s\n' "$*" >>"$CALLS"
exit 17
STUB
  export CALLS="$PWD/calls"
  cat >bin/agentrepo <<'STUB'
#!/bin/sh
if [ "$2" = --capability ]; then
  case "$PROBE" in
  older) echo 'unknown command "generate-skills" for "agentrepo"' >&2; exit 5 ;;
  broken) echo generate-skills-v1; exit 23 ;;
  malformed) echo unexpected; exit 0 ;;
  *) echo generate-skills-v1; exit 0 ;;
  esac
fi
printf 'go:%s\n' "$*" >>"$CALLS"
touch partial-output
exit 23
STUB
  chmod +x bin/agentrepo
  for probe in older broken malformed supported; do
    : >"$CALLS"
    run env PATH="$PWD/bin:$PATH" PROBE="$probe" bash adapter/generate-skills.sh --check extra
    case "$probe" in
    older) [ "$status" -eq 17 ]; [ "$(cat "$CALLS")" = 'fallback:--check extra' ] ;;
    supported) [ "$status" -eq 23 ]; [ "$(cat "$CALLS")" = 'go:generate-skills --check extra' ]; [ -f partial-output ] ;;
    *) [ "$status" -ne 0 ]; [ ! -s "$CALLS" ] ;;
    esac
  done
  # A PATH containing only the shell and dirname represents no installed binary.
  rm bin/agentrepo
  ln -s "$(command -v bash)" bin/bash
  ln -s "$(command -v dirname)" bin/dirname
  : >"$CALLS"
  run env PATH="$PWD/bin" bash adapter/generate-skills.sh --check extra
  [ "$status" -eq 17 ]
  [ "$(cat "$CALLS")" = 'fallback:--check extra' ]
}

@test "source discovery follows directory links and skips dot directories" {
  ln -s demo packages/agents/source/skills/alias
  cp -pR "$src" packages/agents/source/skills/.hidden
  run bash "$FROZEN"
  [ "$status" -eq 0 ]
  expected=$output
  run agentrepo generate-skills
  [ "$status" -eq 0 ]
  [ "$output" = "$expected" ]
}

@test "Go and frozen generation skip symlinked content roots" {
  mkdir -p "$src/codex/scripts"
  printf 'codex content\n' >"$src/codex/scripts/only.sh"
  for tree in shared claude codex; do
    mv "$src/$tree" "$REPO/$tree-content"
    ln -s "$REPO/$tree-content" "$src/$tree"
  done
  run bash "$FROZEN"
  [ "$status" -eq 0 ]
  expected=$output
  cp -pR packages/agents/claude expected-claude
  cp -pR packages/agents/codex expected-codex
  rm -rf packages/agents/claude packages/agents/codex
  run agentrepo generate-skills
  [ "$status" -eq 0 ]
  [ "$output" = "$expected" ]
  diff -r expected-claude packages/agents/claude
  diff -r expected-codex packages/agents/codex
  [ ! -e "$CLAUDE_OUT/scripts" ]
  [ ! -e "$CODEX_OUT/scripts" ]
  [ ! -x "$CLAUDE_OUT/SKILL.md" ]
  [ ! -x "$CODEX_OUT/SKILL.md" ]
}

@test "repository discovery preserves Git status and streams outside a repository" {
  bats_require_minimum_version 1.5.0
  mkdir "$BATS_TEST_TMPDIR/outside"
  cd "$BATS_TEST_TMPDIR/outside"
  export GIT_CEILING_DIRECTORIES="$BATS_TEST_TMPDIR"
  run --separate-stderr bash "$FROZEN"
  [ "$status" -eq 128 ]
  expected_status=$status
  expected_stdout=$output
  expected_stderr=$stderr
  run --separate-stderr agentrepo generate-skills
  [ "$status" -eq "$expected_status" ]
  [ "$output" = "$expected_stdout" ]
  [ "$stderr" = "$expected_stderr" ]
  run --separate-stderr bash "$GEN"
  [ "$status" -eq "$expected_status" ]
  [ "$output" = "$expected_stdout" ]
  [ "$stderr" = "$expected_stderr" ]
}

@test "space-containing output names retain frozen findings and totals" {
  bash "$GEN"
  printf extra >"$CLAUDE_OUT/extra one"
  printf extra >"$CLAUDE_OUT/extra two"
  run bash "$FROZEN" --check
  [ "$status" -eq 1 ]
  expected=$output
  run agentrepo generate-skills --check
  [ "$status" -eq 1 ]
  [ "$output" = "$expected" ]
}

@test "a file colliding with an output directory retains the frozen diagnostic" {
  rm -rf "$src/claude/scripts"
  printf collision >"$src/claude/scripts"
  run bash "$FROZEN"
  [ "$status" -eq 1 ]
  expected=$output
  run agentrepo generate-skills
  [ "$status" -eq 1 ]
  [ "$output" = "$expected" ]
}
