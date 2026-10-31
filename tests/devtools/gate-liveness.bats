#!/usr/bin/env bats

setup_file() {
  ROOT=$(cd "$BATS_TEST_DIRNAME/../.." && pwd)
  export ROOT
  export LIVENESS_GUARD="$BATS_FILE_TMPDIR/guard-markdown"
  cd "$ROOT"
  go build -o "$LIVENESS_GUARD" ./cmd/guard-markdown
}

setup() {
  export REPO="$BATS_TEST_TMPDIR/consumer"
  export CACHE="$BATS_TEST_TMPDIR/shared-cache"
  export TEMPORARY="$BATS_TEST_TMPDIR/temporary"
  mkdir -p "$REPO/.devtools/scripts" "$REPO/.devtools/tasks" "$REPO/.vale/probe" "$CACHE" "$TEMPORARY"
  cp "$ROOT/.devtools/scripts/check_gate_liveness.py" "$ROOT/.devtools/scripts/check_curl_fail.py" "$REPO/.devtools/scripts/"
  sed -n '/^\["devtools:check-gate-liveness"\]/,$p' "$ROOT/.devtools/tasks/lint.toml" >"$REPO/.devtools/tasks/lint.toml"
  printf '# Probe\n\nA plain sentence.\n' >"$REPO/base.md"
  printf '#!/bin/sh\necho yes\n' >"$REPO/base.sh"
  printf 'StylesPath = .vale\nMinAlertLevel = warning\n[*.md]\nBasedOnStyles = probe\n' >"$REPO/.vale.ini"
  printf 'extends: existence\nmessage: Avoid this phrase.\nlevel: error\ntokens:\n  - very unique\n' >"$REPO/.vale/probe/Unique.yml"
  printf '.vale/\n' >"$REPO/.gitignore"
  printf unchanged >"$CACHE/sentinel"
  git -C "$REPO" init --quiet
}

configure() {
  cd "$ROOT"
  uv run python - "$REPO" "$LIVENESS_GUARD" "$1" <<'PY'
import json
import shlex
import subprocess
import sys
from pathlib import Path

repo, guard, mode = sys.argv[1:]
gates = {
    "lint-prose": "vale $(git ls-files '*.md')",
    "devtools:lint-markdown": "rumdl check .",
    "lint-markdown-wrap": "guard-markdown $(git ls-files '*.md')",
    "lint-shell": "shellcheck $(git ls-files '*.sh')",
    "lint-shell-fmt": "shfmt -d $(git ls-files '*.sh')",
    "devtools:lint-curl": 'uv run --script "$DEVTOOLS_DIR/scripts/check_curl_fail.py" $(git ls-files "*.sh")',
}
if mode == "narrow":
    gates["lint-prose"] = "vale base.md"
if mode == "partial":
    gates = {"lint-shell": gates["lint-shell"]}
config = '[task_config]\nincludes = [".devtools/tasks/lint.toml"]\n'
config += '[env]\nDEVTOOLS_DIR = "{{config_root}}/.devtools"\n'
for gate, command in gates.items():
    tool, args = command.split(" ", 1)
    executable = guard if tool == "guard-markdown" else subprocess.check_output(
        ["mise", "which", tool], text=True
    ).strip()
    resolved = f"{shlex.quote(executable)} {args}"
    config += f'\n[tasks.{json.dumps(gate)}]\nrun = {json.dumps(resolved)}\n'
Path(repo, "mise.toml").write_text(config)
PY
  cd "$REPO"
  git add -- base.md base.sh .devtools .vale.ini .gitignore mise.toml
  before_status=$(git status --porcelain)
  before_index=$(git ls-files --stage)
}

check_liveness() {
  run env MISE_TRUSTED_CONFIG_PATHS="$REPO" UV_CACHE_DIR="$CACHE" XDG_CACHE_HOME="$CACHE" TMPDIR="$TEMPORARY" mise run devtools:check-gate-liveness
}

assert_clean() {
  [ "$(git status --porcelain)" = "$before_status" ]
  [ "$(git ls-files --stage)" = "$before_index" ]
  [ ! -e "$REPO/gate-liveness" ]
  for path in "$TEMPORARY"/gate-liveness-*; do
    [ ! -e "$path" ]
  done
  [ "$(cat "$CACHE/sentinel")" = unchanged ]
  ! grep -R -q 'gate-liveness/nested/probe' "$CACHE"
  [ -z "$(git rev-list --all)" ]
}

@test "AC1 AC3 AC5: published task rejects nested defects with exact gate membership" {
  configure all
  check_liveness
  [ "$status" -eq 0 ]
  [[ "$output" != *"SKIP "* ]]
  actual=$(printf '%s\n' "$output" | sed -n 's/^PASS \(.*\): rejected.*/\1/p' | sort)
  expected=$(printf '%s\n' lint-prose devtools:lint-markdown lint-markdown-wrap lint-shell lint-shell-fmt devtools:lint-curl | sort)
  [ "$actual" = "$expected" ]
  assert_clean
}

@test "AC2 AC3: narrowed selection fails with its gate and nested path and cleans up" {
  configure narrow
  check_liveness
  [ "$status" -ne 0 ]
  [[ "$output" == *"FAIL lint-prose: did not reject gate-liveness/nested/probe.md"* ]]
  assert_clean
}

@test "AC4: partial consumers cover defined tasks and name every skipped gate" {
  configure partial
  check_liveness
  [ "$status" -eq 0 ]
  [[ "$output" == *"PASS lint-shell: rejected gate-liveness/nested/probe.sh"* ]]
  for gate in lint-prose devtools:lint-markdown lint-markdown-wrap lint-shell-fmt devtools:lint-curl; do
    [[ "$output" == *"SKIP $gate: task not defined"* ]]
  done
  assert_clean
}
