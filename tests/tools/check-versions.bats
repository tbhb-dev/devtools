#!/usr/bin/env bats
#
# Tests for tools/check-versions.sh, the witness on cog.toml's
# pre_bump_hooks.
#
# Each test builds a throwaway repository carrying the same eleven sites
# the real tree does, because the script asks git for the tag and reads
# the files relative to the toplevel. Running it against this repository
# instead would only ever prove the aligned case, and the cases worth
# holding are the failures: a literal left behind, and a site that has
# moved out from under its hook.

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  SCRIPT="${BATS_TEST_DIRNAME}/../../tools/check-versions.sh"
  REPO="${BATS_TEST_TMPDIR}/repo"

  # Cut the operator's configuration out entirely. An identity has to
  # come from somewhere once the global file is gone, and tag.gpgSign or
  # tag.forceSignAnnotated would otherwise turn the lightweight `git tag`
  # below into an annotated one and stop for a message.
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
  export GIT_AUTHOR_NAME='Test' GIT_AUTHOR_EMAIL='test@example.com'
  export GIT_COMMITTER_NAME='Test' GIT_COMMITTER_EMAIL='test@example.com'

  mkdir -p "$REPO/packages/devtools" "$REPO/packages/agents/common" \
    "$REPO/packages/agents/claude" "$REPO/packages/agents/codex" \
    "$REPO/packages/agents/agy"
  cd "$REPO"
  git init --quiet --initial-branch=main .

  # Use the real working-tree run task and Go sources in every fixture.
  SOURCE=$(cd "$BATS_TEST_DIRNAME/../.." && pwd)
  awk '
    /^\[/ { copy = ($0 == "[tasks.run]" || $0 == "[tasks.check-versions]") }
    copy { print }
  ' "$SOURCE/mise.toml" >mise.toml
  # Keep nested mise runs on the pinned Go rather than the runner's Go,
  # whose first toolchain download would break the silence assertion.
  awk '$1 == "go" && $2 == "=" { print "[tools]"; print }' \
    "$SOURCE/.config/mise/conf.d/10-devtools-tools.toml" >>mise.toml
  cp "$SOURCE/.config/mise/mise.lock" mise.lock
  mkdir -p tools
  cp "$SCRIPT" tools/check-versions.sh
  cp "$SOURCE/tools/go-ldflags.sh" tools/go-ldflags.sh
  local path
  for path in cmd internal vendor go.mod go.sum .devtools; do
    ln -s "$SOURCE/$path" "$path"
  done
  export MISE_TRUSTED_CONFIG_PATHS="$REPO"
}

# The eleven sites at $1, laid out the way the real files carry them:
# apm.yml, the four sub-package apm.yml files, and pyproject.toml
# anchored at line start, the hook manifest's inside an indented
# comment, the README's three under surrounding prose, and uv.lock's
# under the package entry that names it. The lockfile gets a decoy
# package carrying its own version line, because that is what stops
# the check from matching the first `version =` it finds.
write_sites() {
  local v=$1
  : > apm.lock.yaml
  for package in common claude codex agy; do
    printf -- '- repo_url: _local/%s\n  version: %s\n  source: local\n  local_path: ./packages/agents/%s\n' "$package" "$v" "$package" >> apm.lock.yaml
  done
  printf 'name: devtools\nversion: %s\n' "$v" > apm.yml
  printf 'name: devtools-agents-common\nversion: %s\n' "$v" > packages/agents/common/apm.yml
  printf 'name: devtools-agents-claude\nversion: %s\n' "$v" > packages/agents/claude/apm.yml
  printf 'name: devtools-agents-codex\nversion: %s\n' "$v" > packages/agents/codex/apm.yml
  printf 'name: devtools-agents-agy\nversion: %s\n' "$v" > packages/agents/agy/apm.yml
  printf '[project]\nname = "devtools"\nversion = "%s"\n' "$v" > packages/devtools/pyproject.toml
  printf '# Reference it with:\n#\n#     rev: v%s\n' "$v" > .pre-commit-hooks.yaml
  printf '# devtools\n\n    apm install tbhb-dev/devtools#v%s\n\n    apm install tbhb-dev/devtools/packages/agents/claude#v%s\n\nAnd:\n\n    rev: v%s\n' "$v" "$v" "$v" > README.md
  printf '[[package]]\nname = "pytest"\nversion = "9.9.9"\n\n[[package]]\nname = "devtools"\nversion = "%s"\nsource = { editable = "packages/devtools" }\n' "$v" > uv.lock
}

commit_at() {
  local v=$1
  write_sites "$v"
  git add -A
  git commit --quiet -m "release v$v"
  git tag "v$v"
}

@test "passes when every literal names the latest tag" {
  commit_at 1.2.3
  run "$SCRIPT"
  [ "$status" -eq 0 ]
  [ -z "$output" ]
}

@test "fails naming the one literal left behind" {
  commit_at 1.2.3
  # The shape a half-applied bump leaves: four sites rewritten, one not.
  printf '[project]\nname = "devtools"\nversion = "1.2.2"\n' > packages/devtools/pyproject.toml

  run "$SCRIPT"
  [ "$status" -eq 1 ]
  [[ $output == *"packages/devtools/pyproject.toml:3"* ]]
  [[ $output == *'expected `version = "1.2.3"`'* ]]
  [[ $output == *'found `version = "1.2.2"`'* ]]
  [[ $output == *"TOTAL: 1 finding(s)"* ]]
}

@test "fails naming a stale sub-package apm.yml literal" {
  commit_at 1.2.3
  printf 'name: devtools-agents-claude\nversion: 1.2.2\n' > packages/agents/claude/apm.yml

  run "$SCRIPT"
  [ "$status" -eq 1 ]
  [[ $output == *"packages/agents/claude/apm.yml:2"* ]]
  [[ $output == *'expected `version: 1.2.3`'* ]]
  [[ $output == *'found `version: 1.2.2`'* ]]
  [[ $output == *"TOTAL: 2 finding(s)"* ]]
}

@test "reports each README site independently" {
  commit_at 1.2.3
  printf '# devtools\n\n    apm install tbhb-dev/devtools#v1.2.3\n\n    apm install tbhb-dev/devtools/packages/agents/claude#v1.2.3\n\nAnd:\n\n    rev: v0.9.0\n' > README.md

  run "$SCRIPT"
  [ "$status" -eq 1 ]
  [[ $output == *"README.md:9"* ]]
  [[ $output != *"README.md:3"* ]]
  [[ $output != *"README.md:5"* ]]
  [[ $output == *"TOTAL: 1 finding(s)"* ]]
}

@test "fails when a site is deleted rather than left stale" {
  commit_at 1.2.3
  # A grep for the expected string alone would read this as a pass, which
  # is the whole reason the shape is matched separately from the value.
  printf '# Reference it with:\n#\n#     see the README\n' > .pre-commit-hooks.yaml

  run "$SCRIPT"
  [ "$status" -eq 1 ]
  [[ $output == *".pre-commit-hooks.yaml:1"* ]]
  [[ $output == *"no line matching"* ]]
}

@test "reads the lockfile version under the devtools package alone" {
  commit_at 1.2.3
  # The decoy still names 9.9.9 and the devtools entry falls behind. A
  # check reading the first version line in the file would report the
  # decoy instead, or miss this entirely.
  printf '[[package]]\nname = "pytest"\nversion = "9.9.9"\n\n[[package]]\nname = "devtools"\nversion = "1.2.2"\nsource = { editable = "packages/devtools" }\n' > uv.lock

  run "$SCRIPT"
  [ "$status" -eq 1 ]
  [[ $output == *"uv.lock:7"* ]]
  [[ $output == *'found `version = "1.2.2"`'* ]]
  [[ $output == *"TOTAL: 1 finding(s)"* ]]
}

@test "fails when the lockfile drops the devtools package" {
  commit_at 1.2.3
  printf '[[package]]\nname = "pytest"\nversion = "9.9.9"\n' > uv.lock

  run "$SCRIPT"
  [ "$status" -eq 1 ]
  [[ $output == *"uv.lock:1"* ]]
  [[ $output == *"no package entry named devtools"* ]]
}

@test "an explicit version overrides the tag" {
  commit_at 1.2.3

  run "$SCRIPT" 1.2.3
  [ "$status" -eq 0 ]

  run "$SCRIPT" 9.9.9
  [ "$status" -eq 1 ]
  [[ $output == *"TOTAL: 11 finding(s) against v9.9.9"* ]]
}

@test "an explicit version tolerates a leading v" {
  commit_at 1.2.3
  run "$SCRIPT" v1.2.3
  [ "$status" -eq 0 ]
}

@test "fails rather than passing where no tag is reachable" {
  write_sites 1.2.3
  git add -A
  git commit --quiet -m "no tag here"

  run "$SCRIPT"
  [ "$status" -eq 1 ]
  [[ $output == *"no tag reachable from HEAD"* ]]
}

@test "fails when a local APM lock version drifts from its manifest" {
  commit_at 1.2.3
  awk '
    !changed && /version: 1[.]2[.]3/ {
      sub(/version: 1[.]2[.]3/, "version: 1.2.2")
      changed = 1
    }
    { print }
  ' apm.lock.yaml > changed
  mv changed apm.lock.yaml

  run "$SCRIPT"
  [ "$status" -eq 1 ]
  [[ $output == *"apm.lock.yaml:2"* ]]
  [[ $output == *'expected `version: 1.2.3`'* ]]
  [[ $output == *"TOTAL: 1 finding(s)"* ]]
}

@test "fails when a local APM lock entry is missing" {
  commit_at 1.2.3
  sed '/local_path: .\/packages\/agents\/common/d' apm.lock.yaml > changed
  mv changed apm.lock.yaml

  run "$SCRIPT"
  [ "$status" -eq 1 ]
  [[ $output == *"no local package entry for packages/agents/common"* ]]
}

@test "mise task runs the real shim on consistent and inconsistent trees" {
  commit_at 1.2.3
  run mise --quiet run check-versions
  [ "$status" -eq 0 ]
  [ -z "$output" ]

  printf 'version = "1.2.2"\n' >packages/devtools/pyproject.toml
  run mise --quiet run check-versions
  [ "$status" -ne 0 ]
  [[ $output == *'version-literal-stale'* ]]
}

@test "readiness reaches the real shim on consistent and inconsistent trees" {
  commit_at 1.2.3
  local origin="$BATS_TEST_TMPDIR/origin.git" bin="$BATS_TEST_TMPDIR/bin"
  git init --quiet --bare --initial-branch=main "$origin"
  git remote add origin "$origin"
  git push --quiet origin main
  mkdir -p "$bin"
  printf '#!/bin/sh\nprintf "success build\\n"\n' >"$bin/gh"
  # Only the changelog is synthetic; run and check-versions remain real.
  cat >>mise.toml <<'TASK'

[tasks.preview-changelog]
run = 'echo "- a changelog entry"'
TASK
  git add mise.toml
  git commit --quiet -m 'fixture preview'
  git push --quiet origin main
  chmod +x "$bin/gh"
  export PATH="$bin:$PATH"

  run bash "$SOURCE/tools/release-readiness.sh"
  [ "$status" -eq 0 ]
  [[ $output == *'OK    version literals name the latest tag'* ]]

  printf 'version = "1.2.2"\n' >packages/devtools/pyproject.toml
  run bash "$SOURCE/tools/release-readiness.sh"
  [ "$status" -eq 1 ]
  [[ $output == *'FAIL  version literals name the latest tag'* ]]
  [[ $output == *'version-literal-stale'* ]]
}

@test "postflight reaches the real shim for the latest tag on both trees" {
  commit_at 1.2.3
  git tag -d v1.2.3
  git tag -a v1.2.3 -m v1.2.3
  local origin="$BATS_TEST_TMPDIR/origin.git" bin="$BATS_TEST_TMPDIR/bin"
  git init --quiet --bare --initial-branch=main "$origin"
  git remote add origin "$origin"
  git push --quiet origin main refs/tags/v1.2.3
  git fetch --quiet origin
  mkdir -p "$bin"
  printf '#!/bin/sh\nprintf "true valid\\n"\n' >"$bin/gh"
  # Supply only the unrelated signature metadata. Git discovery, refs,
  # reads used by the checker, the shim and the Go build all remain real.
  export REAL_GIT
  REAL_GIT=$(command -v git)
  cat >"$bin/git" <<'GIT'
#!/bin/sh
case "$1 $2" in
  'cat-file tag')
    printf '%s\n' 'tagger tbhb.dev Agent <334376894+tbhb-agent@users.noreply.github.com> 0 +0000' '-----BEGIN SSH SIGNATURE-----'
    ;;
  'cat-file commit')
    printf '%s\n' 'author tbhb-dev-releases[bot] <340658377+tbhb-dev-releases[bot]@users.noreply.github.com> 0 +0000' 'gpgsig fixture'
    ;;
  *) exec "$REAL_GIT" "$@" ;;
esac
GIT
  chmod +x "$bin/gh" "$bin/git"
  export PATH="$bin:$PATH"

  run bash "$SOURCE/tools/release-verify.sh"
  [ "$status" -eq 0 ]
  [[ $output == *'OK    version literals name v1.2.3'* ]]
  [[ $output != *'FAIL  version literals name'* ]]

  printf 'version = "1.2.2"\n' >packages/devtools/pyproject.toml
  run bash "$SOURCE/tools/release-verify.sh"
  [ "$status" -eq 1 ]
  [[ $output == *'FAIL  version literals name v1.2.3'* ]]
  [[ $output == *'version-literal-stale'* ]]
}

@test "shim fails closed when the working-tree build path is unavailable" {
  commit_at 1.2.3
  rm go.mod
  run "$SCRIPT"
  [ "$status" -ne 0 ]
  [[ $output != *'TOTAL:'* ]]
}
