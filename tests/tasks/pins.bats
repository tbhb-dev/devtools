#!/usr/bin/env bats
# Extract the shipped task bodies so the tests exercise the consumer payload.

load ../test_helper

setup() {
  isolate_git
  TASKS="${BATS_TEST_DIRNAME}/../../.devtools/tasks/pins.toml"
  WORK="${BATS_TEST_TMPDIR}/consumer"
  STUB="${BATS_TEST_TMPDIR}/bin"
  mkdir -p "$WORK" "$STUB"
  cp -R "${BATS_TEST_DIRNAME}/fixtures/pins/." "$WORK/"
  cp "${BATS_TEST_DIRNAME}/../../.devtools/vendir.yml" "$WORK/vendir.yml"
  extract_task devtools:update-pins > "${BATS_TEST_TMPDIR}/update.sh"
  extract_task devtools:check-pins > "${BATS_TEST_TMPDIR}/check.sh"
  export REAL_GIT
  REAL_GIT=$(command -v git)
  cd "$WORK"
  "$REAL_GIT" init -q
  "$REAL_GIT" add -- mise.toml apm.yml vendir.yml .github
  cat > "$STUB/git" <<'STUB'
#!/usr/bin/env bash
if [ "$1" != ls-remote ]; then
  exec "$REAL_GIT" "$@"
fi
printf 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\trefs/tags/v0.11.0^{}\n'
STUB
  cat > "$STUB/mise" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$MISE_CALLS"
if [ "$*" = 'run devtools:apm-sync' ]; then
  echo "apm-sync status ${APM_STATUS:-0}"
  exit "${APM_STATUS:-0}"
elif [ "$*" = install ]; then
  echo "install status ${INSTALL_STATUS:-0}"
  exit "${INSTALL_STATUS:-0}"
elif [ "$*" = 'tasks ls --all' ]; then
  echo 'devtools:check-pins '
elif [ "$*" = 'run devtools:check-pins' ]; then
  bash "$CHECK_SCRIPT"
fi
STUB
  cat > "$STUB/vendir" <<'STUB'
#!/usr/bin/env bash
if [ -n "${DELIVERED_CHECK:-}" ]; then
  cp "$DELIVERED_CHECK" "$CHECK_SCRIPT"
fi
echo "sync status ${SYNC_STATUS:-0}"
exit "${SYNC_STATUS:-0}"
STUB
  chmod +x "$STUB/"*
  export PATH="$STUB:$PATH"
  export MISE_CALLS="${BATS_TEST_TMPDIR}/mise-calls"
  export CHECK_SCRIPT="${BATS_TEST_TMPDIR}/check.sh"
  cd "$WORK"
}

extract_task() {
  awk -v task="$1" '
    $0 == "[\"" task "\"]" { selected = 1; next }
    selected && /^run = / { body = 1; next }
    body && /^\047\047\047$/ { exit }
    body { print }
  ' "$TASKS"
}

@test "update moves presets and both whole-payload vendir refs" {
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  [ "$(grep -c 'json5#bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", // v0.11.0' .github/renovate.json5)" -eq 2 ]
  [ "$(grep -c 'ref: v0.11.0' vendir.yml)" -eq 2 ]
  grep -Fq '.devtools/**' vendir.yml
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 0 ]
  [[ "$output" == *'every devtools pin is on v0.11.0'* ]]
}

@test "check rejects drift when only the general preset moved" {
  sed 's/renovate-config.json5#aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/renovate-config.json5#bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/' \
    .github/renovate.json5 > changed
  mv changed .github/renovate.json5
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 1 ]
  [[ "$output" == *'pins disagree on the commit'* ]]
  [[ "$output" == *'.github/renovate.json5:4'* ]]
}

@test "unpinned presets remain unpinned and check reports version disagreement" {
  sed 's/renovate-go-config.json5#[a-f]*/renovate-go-config.json5/' \
    .github/renovate.json5 > changed
  mv changed .github/renovate.json5
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 1 ]
  grep -q 'renovate-go-config.json5"' .github/renovate.json5
  [[ "$output" == *'unpinned'* ]]
  [[ "$output" == *'pins disagree on the version'* ]]
}

@test "any named devtools preset participates in updating and checking" {
  sed 's/renovate-go-config/custom-preset/' .github/renovate.json5 > changed
  mv changed .github/renovate.json5
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  grep -q 'custom-preset.json5#bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", // v0.11.0' .github/renovate.json5
  sed 's/custom-preset.json5#[a-f]*/custom-preset.json5#aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/' \
    .github/renovate.json5 > changed
  mv changed .github/renovate.json5
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 1 ]
  [[ "$output" == *'pins disagree on the commit'* ]]
}

@test "update moves bare and quoted harness pins with the required tool versions" {
  cat > apm.yml <<'YAML'
# tbhb-dev/devtools adoption example
dependencies:
  apm:
    - tbhb-dev/devtools#v0.10.0
    - tbhb-dev/devtools/packages/agents/common#v0.10.0
    - tbhb-dev/devtools/packages/agents/agy#v0.10.0
    - "tbhb-dev/devtools/packages/agents/claude#v0.10.0" # package pin
    - 'tbhb-dev/devtools/packages/agents/codex#v0.10.0'
YAML
  run env usage_version=0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  [ "$(grep -c '#v0.11.0' apm.yml)" -eq 5 ]
  grep -Fq 'codex#v0.11.0'\' apm.yml
  grep -Fq 'guard-markdown" = "v0.11.0"' mise.toml
  grep -Fq 'commit-lint" = "v0.11.0"' mise.toml
}

@test "check rejects guard version drift" {
  sed 's/v0.10.0/v0.9.0/' mise.toml > changed
  mv changed mise.toml
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 1 ]
  [[ "$output" == *'pins disagree on the version'* ]]
}

@test "check and update reject unsupported sub-packages before mutation" {
  printf '  - tbhb-dev/devtools/packages/unknown#v0.10.0\n' > apm.yml
  cp apm.yml before
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 1 ]
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'unsupported devtools APM pin'* ]]
  cmp before apm.yml
}

@test "check and update require a managed guard pin" {
  printf '[tools]\n' > mise.toml
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 1 ]
  [[ "$output" == *'missing guard-markdown'* ]]
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'declare '* ]]
}

@test "check and update require a managed commit-lint pin" {
  sed '/cmd\/commit-lint/d' mise.toml >changed
  mv changed mise.toml
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 1 ]
  [[ "$output" == *'"go:github.com/tbhb-dev/devtools/cmd/commit-lint" = "v0.10.0"'* ]]
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'"go:github.com/tbhb-dev/devtools/cmd/commit-lint" = "v0.10.0"'* ]]
}

@test "update calls shared APM reconciliation and stops when it fails" {
  printf '# adapted from tbhb-dev/devtools v0.10.0\n' > tombi.toml
  git add -- tombi.toml
  run env APM_STATUS=1 usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'apm-sync status 1'* ]]
  grep -Fxq '# adapted from tbhb-dev/devtools v0.11.0' tombi.toml
  grep -Fxq 'run devtools:apm-sync' "$MISE_CALLS"
  ! grep -Fxq 'run devtools:check-pins' "$MISE_CALLS"
}

@test "optional agentrepo pin updates and participates in drift checks" {
  printf '\n"go:github.com/tbhb-dev/devtools/cmd/agentrepo" = "v0.9.0"\n' >> mise.toml
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 1 ]
  [[ "$output" == *'pins disagree on the version'* ]]
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  grep -Fq 'agentrepo" = "v0.11.0"' mise.toml
}

@test "missing agentrepo pin recommends without failing or inserting a pin" {
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 0 ]
  [[ "$output" == *'recommend declaring'*'cmd/agentrepo'* ]]
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  ! grep -q 'cmd/agentrepo' mise.toml
}

@test "agentrepo pin does not replace the required guard pin" {
  sed 's/guard-markdown/agentrepo/' mise.toml > changed
  mv changed mise.toml
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 1 ]
  [[ "$output" == *'missing guard-markdown'* ]]
}

@test "check accepts a matching tracked prose mention" {
  printf '# adapted from tbhb-dev/devtools v0.10.0.\n' > tombi.toml
  git add -- tombi.toml
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 0 ]
}

@test "check rejects every stale prose mention with its location and pin" {
  printf '# adapted from tbhb-dev/devtools v0.9.0 and tbhb-dev/devtools v0.8.0.\n' > tombi.toml
  git add -- tombi.toml
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 1 ]
  [[ "$output" == *'tombi.toml:1'*'v0.9.0'*'pinned v0.10.0'* ]]
  [[ "$output" == *'tombi.toml:1'*'v0.8.0'*'pinned v0.10.0'* ]]
}

@test "check accepts no prose mention and ignores untracked mentions" {
  printf '# adapted from tbhb-dev/devtools v0.9.0\n' > untracked.toml
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 0 ]
}

@test "update rewrites prose naming the previous pin and reports its location" {
  printf '# adapted from tbhb-dev/devtools v0.10.0\n' > tombi.toml
  printf '#!/usr/bin/env bash\n# tbhb-dev/devtools v0.10.0\n' > "consumer script"
  chmod +x "consumer script"
  git add -- tombi.toml "consumer script"
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  grep -Fxq '# adapted from tbhb-dev/devtools v0.11.0' tombi.toml
  [ -x "consumer script" ]
  grep -Fxq '# tbhb-dev/devtools v0.11.0' "consumer script"
  [[ "$output" == *'consumer script:2: prose v0.10.0 -> v0.11.0'* ]]
  [[ "$output" == *'tombi.toml:1: prose v0.10.0 -> v0.11.0'* ]]
}

@test "update rewrites prose in a tracked Unicode filename" {
  file=$'r\303\251sum\303\251.md'
  printf '# adapted from tbhb-dev/devtools v0.10.0\n' > "$file"
  git add -- "$file"
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  grep -Fxq '# adapted from tbhb-dev/devtools v0.11.0' "$file"
}

@test "update rewrites prose in a tracked filename with a space" {
  file='consumer notes.md'
  printf '# adapted from tbhb-dev/devtools v0.10.0\n' > "$file"
  git add -- "$file"
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  grep -Fxq '# adapted from tbhb-dev/devtools v0.11.0' "$file"
}

@test "update rewrites prose in tracked filenames with quotes and newlines" {
  for file in 'consumer "notes".md' $'consumer\nnotes.md'; do
    printf '# adapted from tbhb-dev/devtools v0.10.0\n' > "$file"
    git add -- "$file"
  done
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  for file in 'consumer "notes".md' $'consumer\nnotes.md'; do
    grep -Fxq '# adapted from tbhb-dev/devtools v0.11.0' "$file"
  done
}

@test "update preserves untracked neighbors named like the old temporary files" {
  printf '# tbhb-dev/devtools v0.10.0\n' > notes.md
  git add -- notes.md
  for file in notes.md mise.toml apm.yml vendir.yml .pre-commit-config.yaml .github/renovate.json5; do
    printf 'unrelated notes for %s\n' "$file" > "$file.pins.tmp"
  done
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  for file in notes.md mise.toml apm.yml vendir.yml .pre-commit-config.yaml .github/renovate.json5; do
    [ "$(cat -- "$file.pins.tmp")" = "unrelated notes for $file" ]
  done
  grep -Fxq '# tbhb-dev/devtools v0.11.0' -- notes.md
}

@test "update rewrites tracked filenames beginning with command options" {
  for file in -n.md --version.md; do
    printf '# tbhb-dev/devtools v0.10.0\n' > "$file"
    git add -- "$file"
  done
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  for file in -n.md --version.md; do
    grep -Fxq '# tbhb-dev/devtools v0.11.0' -- "$file"
    [[ "$output" == *"$file:1: prose v0.10.0 -> v0.11.0"* ]]
  done
}

@test "update skips a tracked symlink without changing its target" {
  target="${BATS_TEST_TMPDIR}/tbhb-devtools-v0.10.0.md"
  printf '# tbhb-dev/devtools v0.10.0\n' > "$target"
  ln -s -- "$target" notes.md
  git add -- notes.md
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  [ -L notes.md ]
  [ "$(readlink -- notes.md)" = "$target" ]
  grep -Fxq '# tbhb-dev/devtools v0.10.0' -- "$target"
  [[ "$output" == *'notes.md: skipped symlink'* ]]
}

@test "update skips a tracked path replaced by a non-regular file" {
  printf '# tbhb-dev/devtools v0.10.0\n' > notes.md
  git add -- notes.md
  rm -- notes.md
  mkdir -- notes.md
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  [ -d notes.md ]
  [[ "$output" == *'notes.md: skipped non-regular file'* ]]
}

@test "update preserves executable prose permissions" {
  printf '#!/usr/bin/env bash\n# tbhb-dev/devtools v0.10.0\n' > executable
  chmod 751 -- executable
  git add -- executable
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  [ "$(stat -c %a -- executable)" = 751 ]
  grep -Fxq '# tbhb-dev/devtools v0.11.0' -- executable
}

@test "update cleans random temporary files after a rewrite failure" {
  cat > "$STUB/sed" <<'STUB'
#!/usr/bin/env bash
if [ "$1" = -nE ]; then
  exec "$REAL_SED" "$@"
fi
exit 19
STUB
  chmod +x -- "$STUB/sed"
  export REAL_SED
  REAL_SED=$(PATH="${PATH#*:}" command -v sed)
  export REAL_MKTEMP
  REAL_MKTEMP=$(command -v mktemp)
  export TEMP_CALLS="${BATS_TEST_TMPDIR}/temp-calls"
  cat > "$STUB/mktemp" <<'STUB'
#!/usr/bin/env bash
created=$("$REAL_MKTEMP" "$@") || exit "$?"
printf '%s\n' "$created" >> "$TEMP_CALLS"
printf '%s\n' "$created"
STUB
  chmod +x -- "$STUB/mktemp"
  cp -p -- apm.yml "${BATS_TEST_TMPDIR}/apm-before"
  run env TMPDIR="$WORK" usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 19 ]
  cmp -- "${BATS_TEST_TMPDIR}/apm-before" apm.yml
  while IFS= read -r created; do
    [ ! -e "$created" ]
  done < "$TEMP_CALLS"
  [[ "$(head -n 1 -- "$TEMP_CALLS")" == /tmp/devtools-update-pins.* ]]
  [ -z "$(find . -name '.pins.*' -print)" ]
}

@test "update preserves other releases for manual edits even beside the previous pin" {
  printf '# tbhb-dev/devtools v0.8.0; tbhb-dev/devtools v0.10.00; tbhb-dev/devtools v0.10.0; tbhb-dev/devtools\tv0.10.0.\n' > tombi.toml
  git add -- tombi.toml
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 1 ]
  grep -Fxq $'# tbhb-dev/devtools v0.8.0; tbhb-dev/devtools v0.10.00; tbhb-dev/devtools v0.11.0; tbhb-dev/devtools\tv0.11.0.' tombi.toml
  [[ "$output" == *'tombi.toml:1: prose mentions v0.8.0; pinned v0.11.0 (edit manually)'* ]]
  [[ "$output" == *'tombi.toml:1: prose mentions v0.10.00; pinned v0.11.0 (edit manually)'* ]]
}

@test "update leaves untracked ignored and binary mentions unchanged" {
  printf '# tbhb-dev/devtools v0.10.0\n' > untracked.toml
  cp untracked.toml ignored.toml
  printf 'ignored.toml\n' > .gitignore
  printf '\0tbhb-dev/devtools v0.10.0\n' > binary.dat
  git add -- .gitignore binary.dat
  cp binary.dat "${BATS_TEST_TMPDIR}/binary-before"
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  grep -Fxq '# tbhb-dev/devtools v0.10.0' untracked.toml
  cmp untracked.toml ignored.toml
  cmp "${BATS_TEST_TMPDIR}/binary-before" binary.dat
}

@test "update respects Git binary attributes when selecting tracked prose" {
  printf 'notes.md binary\n' > .gitattributes
  printf '# tbhb-dev/devtools v0.10.0\n' > notes.md
  git add -- .gitattributes notes.md
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  grep -Fxq '# tbhb-dev/devtools v0.10.0' -- notes.md
}

@test "sync failure stops the update after rewriting prose" {
  printf '# tbhb-dev/devtools v0.10.0\n' > tombi.toml
  git add -- tombi.toml
  run env SYNC_STATUS=1 usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'sync status 1'* ]]
  grep -Fxq '# tbhb-dev/devtools v0.11.0' tombi.toml
  [ ! -f "$MISE_CALLS" ]
}

@test "install failure stops the update after rewriting prose" {
  printf '# tbhb-dev/devtools v0.10.0\n' > tombi.toml
  git add -- tombi.toml
  run env INSTALL_STATUS=1 usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'install status 1'* ]]
  grep -Fxq '# tbhb-dev/devtools v0.11.0' tombi.toml
  ! grep -Fxq 'run devtools:apm-sync' "$MISE_CALLS"
  ! grep -Fxq 'run devtools:check-pins' "$MISE_CALLS"
}

@test "the running updater rewrites prose before sync delivers the final checker" {
  printf '# tbhb-dev/devtools v0.10.0\n' > tombi.toml
  git add -- tombi.toml
  cat > "${BATS_TEST_TMPDIR}/delivered-check.sh" <<'SH'
#!/usr/bin/env bash
grep -Fxq '# tbhb-dev/devtools v0.11.0' tombi.toml || exit 1
echo 'delivered check-pins ran'
SH
  cat > "$CHECK_SCRIPT" <<'SH'
#!/usr/bin/env bash
echo 'old check-pins ran' >&2
exit 1
SH
  run env DELIVERED_CHECK="${BATS_TEST_TMPDIR}/delivered-check.sh" usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  [[ "$output" == *'tombi.toml:1: prose v0.10.0 -> v0.11.0'* ]]
  [[ "$output" == *'delivered check-pins ran'* ]]
  [[ "$output" != *'old check-pins ran'* ]]
}
