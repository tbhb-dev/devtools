#!/usr/bin/env bats

setup() {
  bats_require_minimum_version 1.5.0
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
  REAL_MISE=$(command -v mise)
  export REAL_MISE
  mkdir -p "$BATS_TEST_TMPDIR/bin"
  export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
  cd "$BATS_TEST_TMPDIR"
  cat >bin/mise <<'SH'
#!/usr/bin/env bash
if [[ $1 != which ]]; then exec "$REAL_MISE" "$@"; fi
if [[ -n ${SELECTED:-} ]]; then printf '%s\n' "$SELECTED"; else exit 1; fi
SH
  cat >bin/apm <<'SH'
#!/usr/bin/env bash
printf 'install\n' >>trace
SH
  chmod +x bin/*
}

@test "selected capable binary wins over PATH and Go failures never fall back" {
  cat >selected <<'SH'
#!/usr/bin/env bash
if [[ $* == 'apm-sync --help' ]]; then exit 0; fi
printf 'go\n' >>trace
printf 'partial\n'
printf 'failed\n' >&2
exit 23
SH
  chmod +x selected
  export SELECTED="$PWD/selected"
  run --separate-stderr bash "$ROOT/.devtools/scripts/apm-sync.sh"
  [ "$status" -eq 23 ]
  [ "$output" = partial ]
  [ "$stderr" = failed ]
  [ "$(cat trace)" = go ]
}

@test "unsupported selected binary falls back without trying a newer PATH binary" {
  printf '#!/bin/bash\nexit 1\n' >selected
  printf '#!/bin/bash\ntouch wrong-binary\n' >bin/agentrepo
  chmod +x selected bin/agentrepo
  export SELECTED="$PWD/selected"
  run bash "$ROOT/.devtools/scripts/apm-sync.sh"
  [ "$status" -eq 0 ]
  [ ! -e wrong-binary ]
  [ "$(cat trace)" = install ]
}

@test "missing and old binaries retain the frozen reconciliation path" {
  for mode in missing old; do
    unset SELECTED
    if [[ $mode == old ]]; then
      printf '#!/bin/bash\nexit 1\n' >old
      chmod +x old
      export SELECTED="$PWD/old"
    fi
    # A missing capability on PATH also models an N-2 install.
    printf '#!/bin/bash\nexit 1\n' >bin/agentrepo
    chmod +x bin/agentrepo
    mkdir -p .claude
    printf '{"hooks":{},"local":true}\n' >.claude/settings.json
    run bash "$ROOT/.devtools/scripts/apm-sync.sh"
    [ "$status" -eq 0 ]
    jq -e '.local and (has("hooks") | not)' .claude/settings.json
  done
}

@test "Go and frozen shell preserve streams status and settings across repeated installs" {
  for result in 0 23; do
    export INSTALL_STATUS=$result
    cat >bin/apm <<'SH'
#!/usr/bin/env bash
set -eu
for harness in .claude .agents .codex; do test ! -e "$harness/apm-hooks.json"; done
printf 'install\n' >>trace
printf 'deployment output\n'
printf 'deployment diagnostic\n' >&2
exit "$INSTALL_STATUS"
SH
    chmod +x bin/apm
    for mode in siblings owned unowned empty missing; do
      for implementation in frozen go wrapper repository; do
        dir="$PWD/$result-$mode-$implementation"
        mkdir -p "$dir"
        cd "$dir"
        for spec in '.claude/settings.json hooks' '.agents/hooks.json apm' '.codex/hooks.json hooks'; do
          read -r file key <<<"$spec"
          mkdir -p "${file%/*}"
          touch "${file%/*}/apm-hooks.json"
          case $mode in
            siblings) printf '{"%s":{},"local":true}\n' "$key" >"$file" ;;
            owned) printf '{"%s":{}}\n' "$key" >"$file" ;;
            unowned) printf '{"local":true}\n' >"$file" ;;
            empty) printf '{}\n' >"$file" ;;
          esac
        done
        case $implementation in
          frozen) command=(bash "$ROOT/tests/tools/testdata/apm-sync-frozen.sh") ;;
          go) command=("$ROOT/bin/agentrepo" apm-sync) ;;
          wrapper) export SELECTED="$ROOT/bin/agentrepo"; command=(bash "$ROOT/.devtools/scripts/apm-sync.sh") ;;
          repository)
            export SELECTED="$ROOT/bin/agentrepo"
            mkdir -p .devtools/scripts
            cp "$ROOT/.devtools/scripts/apm-sync.sh" .devtools/scripts/
            uv run --project "$ROOT" --no-sync python - "$ROOT/mise.toml" >task.sh <<'PY'
import sys, tomllib
with open(sys.argv[1], 'rb') as source:
    body = tomllib.load(source)['tasks']['apm-sync']['run']
    print(body[0] if isinstance(body, list) else body)
PY
            command=(bash task.sh) ;;
        esac
        for repetition in first repeated; do
          run --separate-stderr "${command[@]}"
          [ "$status" -eq "$result" ]
          [ "$output" = 'deployment output' ]
          [ "$stderr" = 'deployment diagnostic' ]
        done
        [ "$(cat trace)" = $'install\ninstall' ]
        for file in .claude/settings.json .agents/hooks.json .codex/hooks.json; do
          if [[ $mode == siblings || $mode == unowned ]]; then
            printf '{\n  "local": true\n}\n' >expected
            cmp expected "$file"
          else
            [ ! -e "$file" ]
          fi
        done
        cd "$BATS_TEST_TMPDIR"
      done
    done
  done
}

@test "repository and consumer tasks keep their audit interfaces and stop after failures" {
  export SELECTED="$ROOT/bin/agentrepo"
  export DEVTOOLS_DIR="$ROOT/.devtools"
  export MISE_TRUSTED_CONFIG_PATHS="$BATS_TEST_TMPDIR"
  cat >bin/apm <<'SH'
#!/usr/bin/env bash
printf 'install\n' >>trace
exit "$INSTALL_STATUS"
SH
  cat >bin/uv <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >>trace
exit "$AUDIT_STATUS"
SH
  chmod +x bin/apm bin/uv
  # Read the real task tables; only unrelated repository tasks are omitted.
  for entry in repository consumer; do
    # Use the repository interpreter directly because uv now records audit calls.
    "$ROOT/.venv/bin/python" - "$ROOT" "$entry" >mise.toml <<'PY'
import json, sys, tomllib
from pathlib import Path
root = Path(sys.argv[1])
if sys.argv[2] == 'repository':
    tasks = tomllib.loads((root / 'mise.toml').read_text())['tasks']
    for name in ('apm-sync', 'lint-apm'):
        print('[tasks.' + name + ']')
        for key, value in tasks[name].items():
            print(key + ' = ' + json.dumps(value))
else:
    print('[task_config]\nincludes = [' + json.dumps(str(root / '.devtools/tasks/apm.toml')) + ']')
PY
    mkdir -p .devtools/scripts
    cp "$ROOT/.devtools/scripts/apm-sync.sh" .devtools/scripts/
    task=apm-sync
    [[ $entry == repository ]] || task=devtools:apm-sync
    for stage in install audit success; do
      rm -f trace
      export INSTALL_STATUS=0 AUDIT_STATUS=0
      case $stage in install) INSTALL_STATUS=23 ;; audit) AUDIT_STATUS=24 ;; esac
      run "$REAL_MISE" --quiet run "$task"
      printf "entry=%s stage=%s status=%s trace=%s\n%s\n" "$entry" "$stage" "$status" "$(cat trace)" "$output"
      if [[ $stage == success ]]; then [ "$status" -eq 0 ]; else [ "$status" -ne 0 ]; fi
      if [[ $stage == install ]]; then
        [ "$(cat trace)" = install ]
      elif [[ $entry == repository ]]; then
        [ "$(cat trace)" = $'install\nrun python tools/apm_audit.py' ]
      else
        [ "$(cat trace)" = "$(printf 'install\nrun --script %s/scripts/apm_audit.py' "$DEVTOOLS_DIR")" ]
      fi
    done
  done
}

@test "absent binary uses frozen cleanup before installation" {
  unset SELECTED
  for tool in bash jq mv rm; do
    ln -s "$(command -v "$tool")" "bin/$tool"
  done
  mkdir -p .claude
  printf '{"hooks":{}}\n' >.claude/settings.json
  run env PATH="$PWD/bin" bash "$ROOT/.devtools/scripts/apm-sync.sh"
  [ "$status" -eq 0 ]
  [ ! -e .claude/settings.json ]
  [ "$(cat trace)" = install ]
}
