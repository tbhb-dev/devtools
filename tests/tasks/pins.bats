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
  cat > "$STUB/git" <<'STUB'
#!/usr/bin/env bash
printf 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\trefs/tags/v0.11.0^{}\n'
STUB
  cat > "$STUB/mise" <<'STUB'
#!/usr/bin/env bash
if [ "$*" = 'tasks ls --all' ]; then
  echo 'devtools:check-pins '
elif [ "$*" = 'run devtools:check-pins' ]; then
  bash "$CHECK_SCRIPT"
fi
STUB
  printf '#!/usr/bin/env bash\nexit 0\n' > "$STUB/vendir"
  chmod +x "$STUB/"*
  export PATH="$STUB:$PATH"
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
