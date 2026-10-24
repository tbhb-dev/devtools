#!/usr/bin/env bats
# Extract the shipped task bodies so the tests exercise the consumer payload.

setup() {
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
  exit "${APM_STATUS:-0}"
elif [ "$*" = 'tasks ls --all' ]; then
  echo 'devtools:check-pins '
elif [ "$*" = 'run devtools:check-pins' ]; then
  bash "$CHECK_SCRIPT"
fi
STUB
  printf '#!/usr/bin/env bash\nexit 0\n' > "$STUB/vendir"
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
  grep -Fq 'gh-post" = "v0.11.0"' mise.toml
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
  run env APM_STATUS=1 usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 1 ]
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

@test "update leaves prose for a manual edit reported by check" {
  printf '# adapted from tbhb-dev/devtools v0.10.0\n' > README.md
  git add -- README.md
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 1 ]
  grep -Fxq '# adapted from tbhb-dev/devtools v0.10.0' README.md
  [[ "$output" == *'README.md:1'*'pinned v0.11.0'* ]]
}

@test "update rewrites adapted tombi.toml for the new release" {
  printf '#:schema tombi://www.schemastore.org/tombi.json\n# Standard tombi config (tombi 1.2.0) adapted from tbhb-dev/devtools v0.10.0.\n' > tombi.toml
  git add -- tombi.toml
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  grep -Fxq '# Standard tombi config (tombi 1.2.0) adapted from tbhb-dev/devtools v0.11.0.' tombi.toml
}

@test "check and update require a managed gh-post pin" {
  sed '/cmd\/gh-post/d' mise.toml >changed
  mv changed mise.toml
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 1 ]
  [[ "$output" == *'"go:github.com/tbhb-dev/devtools/cmd/gh-post" = "v0.10.0"'* ]]
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'"go:github.com/tbhb-dev/devtools/cmd/gh-post" = "v0.10.0"'* ]]
}

@test "gh-post drift is checked and moves with the published pins" {
  sed '/cmd\/gh-post/s/v0.10.0/v0.9.0/' mise.toml >changed
  mv changed mise.toml
  run bash "$CHECK_SCRIPT"
  [ "$status" -eq 1 ]
  [[ "$output" == *'pins disagree on the version'* ]]
  run env usage_version=v0.11.0 bash "${BATS_TEST_TMPDIR}/update.sh"
  [ "$status" -eq 0 ]
  grep -Fq 'gh-post" = "v0.11.0"' mise.toml
}
