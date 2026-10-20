#!/usr/bin/env bats

setup() {
  ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
  WORK="${BATS_TEST_TMPDIR}/consumer"
  mkdir -p "$WORK/bin" "$WORK/vendor" "$WORK/.devtools"
  cd "$WORK"
  git init -q
  printf 'module example.com/consumer\n' >go.mod
  printf 'tracked\n' >vendor/modules.txt
  printf 'tracked\n' >.devtools/tracked
  printf '/vendor/ignored\n/.devtools/ignored\n' >.gitignore
  git add -- go.mod vendor/modules.txt .devtools/tracked .gitignore
  printf '#!/usr/bin/env bash\nexit 0\n' >bin/go
  cp bin/go bin/vendir
  chmod +x bin/go bin/vendir
  export PATH="$WORK/bin:$PATH"
  extract_task go devtools:vendor-check >go-check.sh
  extract_task vendir devtools:check-vendored >vendir-check.sh
}

extract_task() {
  awk -v task="$2" '
    $0 == "[\"" task "\"]" { selected = 1; next }
    selected && /^run = / { body = 1; next }
    body && /^\047\047\047$/ { exit }
    body { print }
  ' "$ROOT/.devtools/tasks/$1.toml"
}

@test "Go vendor check rejects and lists an untracked generated file" {
  run bash go-check.sh
  [ "$status" -eq 0 ]
  echo new >vendor/new.go
  run bash go-check.sh
  [ "$status" -eq 1 ]
  [[ "$output" == *'vendor/new.go'* ]]
}

@test "both vendor checks reject files hidden by personal excludes" {
  printf 'hidden\n' >"${BATS_TEST_TMPDIR}/global-ignore"
  git config core.excludesFile "${BATS_TEST_TMPDIR}/global-ignore"
  printf 'local-hidden\n' >.git/info/exclude
  for check in go vendir; do
    if [ "$check" = go ]; then target=vendor; else target=.devtools; fi
    echo new >"$target/hidden"
    echo new >"$target/local-hidden"
    run bash "$check-check.sh"
    [ "$status" -eq 1 ]
    [[ "$output" == *"$target/hidden"* ]]
    [[ "$output" == *"$target/local-hidden"* ]]
  done
}

@test "both vendor checks honor repository ignores and staged payloads" {
  echo ignored >vendor/ignored
  echo ignored >.devtools/ignored
  echo staged >vendor/staged.go
  echo staged >.devtools/staged
  git add -- vendor/staged.go .devtools/staged
  for check in go vendir; do
    run bash "$check-check.sh"
    [ "$status" -eq 0 ]
  done
}

@test "Go vendor check skips modules without a tracked vendor manifest" {
  git rm -q -f -- vendor/modules.txt
  printf '#!/usr/bin/env bash\nexit 99\n' >bin/go
  run bash go-check.sh
  [ "$status" -eq 0 ]
  [[ "$output" == *"skipping module without tracked vendor/modules.txt"* ]]
  [ ! -d vendor ]
}
