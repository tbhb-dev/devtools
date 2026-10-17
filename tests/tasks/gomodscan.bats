#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  ROOT=$(cd "$BATS_TEST_DIRNAME/../.." && pwd)
  local publisher="$BATS_TEST_TMPDIR/publisher"
  mkdir -p "$publisher" "$BATS_TEST_TMPDIR/consumer/bin"
  cp "$ROOT/.pre-commit-hooks.yaml" "$publisher/"
  git -C "$publisher" init -q
  git -C "$publisher" add -- .pre-commit-hooks.yaml
  git -C "$publisher" -c core.hooksPath=/dev/null -c commit.gpgSign=false \
    -c user.name=Test -c user.email=test@example.com commit -qm fixture
  local revision
  revision=$(git -C "$publisher" rev-parse HEAD)
  cd "$BATS_TEST_TMPDIR/consumer"
  git init -q
  git config user.name Test
  git config user.email test@example.com
  git config commit.gpgSign false
  mkdir -p .devtools/tasks vendor
  cp "$ROOT/.devtools/tasks/go.toml" .devtools/tasks/
  cat > mise.toml <<'TOML'
[task_config]
includes = [".devtools/tasks/go.toml"]
TOML
  cat > .pre-commit-config.yaml <<YAML
repos:
  - repo: $publisher
    rev: $revision
    hooks:
      - id: gomodscan
YAML
  cat > bin/go <<'SH'
#!/usr/bin/env bash
set -eu
[ "$#" -eq 2 ]
[ "$1" = tool ]
[ "$2" = gomodscan ]
printf 'scan\n' >> "$SCAN_LOG"
case "$SCAN_RESULT" in
  pass) exit 0 ;;
  fail) echo 'dependency finding' >&2; exit 1 ;;
  unreachable) echo 'connection refused' >&2; exit 2 ;;
esac
SH
  chmod +x bin/go
  export PATH="$PWD/bin:$PATH"
  export MISE_TRUSTED_CONFIG_PATHS="$PWD"
  export SCAN_LOG="$BATS_TEST_TMPDIR/scans" SCAN_RESULT=pass
  prek install
  git add -- mise.toml .pre-commit-config.yaml .devtools/tasks/go.toml bin/go
  git commit -qm setup
}

@test "dependency commits require a passing scan for each dependency path" {
  local path result
  for path in go.mod go.sum vendor/modules.txt; do
    printf 'dependency\n' >> "$path"
    git add -- "$path"
    for result in fail unreachable; do
      export SCAN_RESULT="$result"
      run git commit -qm dependency
      [ "$status" -ne 0 ]
      [ -s "$SCAN_LOG" ]
      if [ "$result" = unreachable ]; then
        [[ "$output" == *"scan could not run"* ]]
      else
        [[ "$output" == *"dependency finding"* ]]
        [[ "$output" != *"scan could not run"* ]]
      fi
      rm "$SCAN_LOG"
    done
    export SCAN_RESULT=pass
    run git commit -qm dependency
    [ "$status" -eq 0 ]
    [ -s "$SCAN_LOG" ]
    rm "$SCAN_LOG"
  done
}

@test "commits without dependency changes skip the scanner" {
  export SCAN_RESULT=unreachable
  printf 'source\n' > main.go
  git add -- main.go
  run git commit -qm source
  [ "$status" -eq 0 ]
  [ ! -e "$SCAN_LOG" ]
}
