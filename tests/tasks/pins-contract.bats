#!/usr/bin/env bats
# Drive the shipped update-pins body through the real agentrepo against a
# fixture release history, so the consumer contract's update-time behavior is
# measured end to end: the contract plan before any edit, the transition
# applied beside the pins, and both readers judging the result.
#
# The history is tests/fixtures/contract: v1.0.0 has no contract, v1.1.0 is
# the baseline, v1.2.0 is the bridge whose reader accepts gamma, and v1.3.0
# requires gamma. The fixture readers are synthetic, so nothing here depends
# on a real release.

setup() {
  ROOT="${BATS_TEST_DIRNAME}/../.."
  AGENTREPO_REAL="$(cd "$ROOT" && pwd)/bin/agentrepo"
  [ -x "$AGENTREPO_REAL" ] || { echo "build bin/agentrepo first" >&2; return 1; }
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
  export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@example.invalid
  export GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@example.invalid
  FIXTURES="${BATS_TEST_DIRNAME}/../fixtures/contract"
  ORIG_PATH="$PATH"
  REMOTE="${BATS_TEST_TMPDIR}/remote"
  STUB="${BATS_TEST_TMPDIR}/bin"
  mkdir -p "$REMOTE" "$STUB"
  build_history
  extract_task devtools:update-pins > "${BATS_TEST_TMPDIR}/update.sh"
  install_stubs
}

# release <tag> <reader> [contract] commits one release into the remote.
release() {
  local tag=$1 reader=$2 contract=${3:-}
  (
    cd "$REMOTE"
    rm -rf .claude .devtools docs
    cp -R "$FIXTURES/$reader/." .
    if [ -n "$contract" ]; then
      mkdir -p .devtools docs
      cp "$FIXTURES/$contract" .devtools/consumer-contract.toml
      "$AGENTREPO_REAL" contract --repo . render
    fi
    git add -A
    git commit -q --allow-empty -m "release $tag"
    git tag "$tag"
  )
}

build_history() {
  git -C "$REMOTE" init -q -b main
  release v1.0.0 reader-v1
  release v1.1.0 reader-v1 contract-baseline.toml
  release v1.2.0 reader-v2 contract-bridge.toml
  release v1.3.0 reader-v3 contract-require.toml
}

# consumer <tag> <conf> checks the remote out at a release, adds the pin
# sites update-pins edits, and gives the consumer its working conf.
consumer() {
  WORK="${BATS_TEST_TMPDIR}/consumer"
  git clone -q "$REMOTE" "$WORK"
  cd "$WORK"
  git checkout -q "$1"
  cp -R "${BATS_TEST_DIRNAME}/fixtures/pins/." .
  cp "${ROOT}/.devtools/vendir.yml" vendir.yml
  mkdir -p .agents
  printf '%s' "$2" > .agents/agent-skills.conf
  git add -A
  git commit -q -m consumer
}

install_stubs() {
  # Resolving the tag is not under test, so ls-remote answers directly.
  cat > "$STUB/git" <<'SH'
#!/usr/bin/env bash
if [ "$1" != ls-remote ]; then
  exec "$REAL_GIT" "$@"
fi
printf 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\trefs/tags/%s\n' "${3#refs/tags/}"
SH
  # The real tool, pointed at the fixture remote and kept off the stubbed
  # PATH, since it runs its own Git.
  cat > "$STUB/agentrepo" <<'SH'
#!/usr/bin/env bash
args=()
for arg in "$@"; do
  [ "$arg" = https://github.com/tbhb-dev/devtools ] && arg=$REMOTE
  args+=("$arg")
done
PATH=$ORIG_PATH exec "$AGENTREPO_REAL" "${args[@]}"
SH
  cat > "$STUB/mise" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$MISE_CALLS"
[ "$*" != 'which agentrepo' ] || printf '%s\n' "$STUB/agentrepo"
SH
  printf '#!/usr/bin/env bash\nexit 0\n' > "$STUB/vendir"
  chmod +x "$STUB/"*
  export REAL_GIT REMOTE ORIG_PATH STUB AGENTREPO_REAL
  REAL_GIT=$(command -v git)
  export MISE_CALLS="${BATS_TEST_TMPDIR}/mise-calls"
  export PATH="$STUB:$PATH"
}

# extract_task prints the body of one task from the shipped task file.
extract_task() {
  local selected="" body="" line
  while IFS= read -r line; do
    if [ -z "$selected" ]; then
      [ "$line" != "[\"$1\"]" ] || selected=1
    elif [ -z "$body" ]; then
      case $line in 'run = '*) body=1 ;; esac
    elif [ "$line" = "'''" ]; then
      return 0
    else
      printf '%s\n' "$line"
    fi
  done < "${BATS_TEST_DIRNAME}/../../.devtools/tasks/pins.toml"
}

update() {
  run env usage_version="$1" bash "${BATS_TEST_TMPDIR}/update.sh"
}

# AC4: after its bridge, the consumer's pins and the declared transition land
# together, and both readers accept the tree.
@test "update to the requiring release writes the pins and the transition together" {
  consumer v1.2.0 $'# keep\nalpha = x\n'
  update v1.3.0
  [ "$status" -eq 0 ]
  grep -Fq 'guard-markdown" = "v1.3.0"' mise.toml
  [ "$(cat .agents/agent-skills.conf)" = $'# keep\nalpha = x\ngamma = on' ]
}

# AC5: a consumer that already satisfies the transition is left alone.
@test "update leaves a conf that already satisfies the transition untouched" {
  consumer v1.2.0 $'alpha   =   x\n\n# a comment\ngamma = on\n'
  cp .agents/agent-skills.conf conf.before
  update v1.3.0
  [ "$status" -eq 0 ]
  cmp conf.before .agents/agent-skills.conf
  grep -Fq 'guard-markdown" = "v1.3.0"' mise.toml
}

# AC10: a consumer whose reader skips the bridge is refused before editing,
# and the refusal names the release to take first.
@test "update refuses a consumer that skips the bridge and edits nothing" {
  consumer v1.1.0 $'alpha = x\n'
  cp mise.toml mise.before
  cp .agents/agent-skills.conf conf.before
  update v1.3.0
  [ "$status" -eq 1 ]
  [[ "$output" == *'v1.2.0'* ]]
  cmp mise.before mise.toml
  cmp conf.before .agents/agent-skills.conf
  ! grep -Fxq 'run devtools:apm-sync' "$MISE_CALLS"
}

# AC10: a consumer that predates the baseline is sent to the baseline.
@test "update refuses a consumer that predates the baseline and names it" {
  consumer v1.0.0 $'alpha = x\n'
  cp mise.toml mise.before
  update v1.2.0
  [ "$status" -eq 1 ]
  [[ "$output" == *'v1.1.0'* ]]
  cmp mise.before mise.toml
}

# The baseline itself is reachable from before the contract existed.
@test "update to the baseline succeeds from a consumer that predates it" {
  consumer v1.0.0 $'alpha = x\n'
  update v1.1.0
  [ "$status" -eq 0 ]
  grep -Fq 'guard-markdown" = "v1.1.0"' mise.toml
}
