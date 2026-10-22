#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  SCRIPT="${BATS_TEST_DIRNAME}/../../.devtools/scripts/gitleaks.sh"
  export NO_COLOR=1
  unset GITLEAKS_CONFIG GITLEAKS_CONFIG_TOML
  git init -q --initial-branch=main "$BATS_TEST_TMPDIR/repo"
  cd "$BATS_TEST_TMPDIR/repo" || return
  git config user.name Test
  git config user.email test@example.com
  git config core.hooksPath /dev/null
  git config commit.gpgSign false
  printf 'Public documentation\n' > tracked
  commit_all
  git update-ref refs/remotes/origin/main HEAD
  git symbolic-ref refs/remotes/origin/HEAD refs/remotes/origin/main
}

commit_all() {
  # Fixture-only staging in an isolated repository.
  git add --all
  git commit -qm fixture
}

seed() {
  SECRET="ghp_$(openssl rand -hex 18)"
  printf 'token=%s\n' "$SECRET" >"${1:-secret.txt}"
}

assert_redacted() {
  [[ "$output" != *"$SECRET"* ]]
  [[ "$output" == *github-pat* ]]
  [[ "$output" == *"${1:-secret.txt}"* ]]
  [[ "$output" =~ Line:[[:space:]]+1 ]]
}

# Compare the state the scanner promises to preserve, including dangling refs.
capture_state() {
  git rev-parse HEAD
  git show-ref
  git ls-files --stage
  git diff --binary
  git status --porcelain=v1 --untracked-files=all
}

@test "AC1 modes are explicit and arguments are strict" {
  run env CI=true GITHUB_ACTIONS=true bash "$SCRIPT"
  [ "$status" -eq 0 ]
  [[ "$output" == *'mode: ci'* ]]
  run env CI=false GITHUB_ACTIONS=false GITLEAKS_MODE=local bash "$SCRIPT"
  [ "$status" -eq 0 ]
  [[ "$output" == *'mode: ci'* ]]
  run env CI=false bash "$SCRIPT" --mode ci
  [ "$status" -eq 0 ]
  [[ "$output" == *'mode: ci'* ]]
  run bash "$SCRIPT" --mode local
  [ "$status" -eq 0 ]
  [[ "$output" == *'mode: local'* ]]
  for mode in wrong LOCAL ''; do
    run bash "$SCRIPT" --mode "$mode"
    [ "$status" -ne 0 ]
    [[ "$output" == *'unknown mode'* ]]
  done
  for arg in --wrong --mode --base; do
    run bash "$SCRIPT" "$arg"
    [ "$status" -ne 0 ]
  done
  run bash "$SCRIPT" --base refs/remotes/origin/main
  [ "$status" -ne 0 ]
  [[ "$output" == *'--base requires --mode local'* ]]
  run bash "$SCRIPT" --mode ci --base refs/remotes/origin/main
  [ "$status" -ne 0 ]
}

@test "AC2 AC6 unrelated local remote and tag refs affect only ci" {
  local ref
  for ref in refs/heads/other refs/remotes/origin/other refs/tags/other; do
    git checkout -q --detach refs/remotes/origin/main
    seed
    commit_all
    local seeded
    seeded=$(git rev-parse HEAD)
    git update-ref "$ref" "$seeded"
    git checkout -q main
    run bash "$SCRIPT" --mode local
    [ "$status" -eq 0 ]
    run bash "$SCRIPT" --mode ci
    [ "$status" -ne 0 ]
    assert_redacted
    git checkout -q --detach "$seeded"
    run bash "$SCRIPT" --mode local
    [ "$status" -ne 0 ]
    assert_redacted
    git checkout -q main
    git update-ref -d "$ref"
  done
}

@test "AC2 AC6 a secret removed from the tip still fails the captured range" {
  seed
  commit_all
  git rm -q secret.txt
  git commit -qm removed
  for mode in local ci; do
    run bash "$SCRIPT" --mode "$mode"
    [ "$status" -ne 0 ]
    assert_redacted
  done
}

@test "AC2 a merge resolution secret removed later remains in the range" {
  git checkout -qb side
  echo side > tracked
  commit_all
  git checkout -q main
  echo main > tracked
  commit_all
  run git merge side -m merge
  [ "$status" -ne 0 ]
  seed tracked
  commit_all
  echo clean > tracked
  commit_all
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  assert_redacted tracked
}

@test "AC2 AC4 AC6 removed base history is excluded locally but unchanged index secrets fail" {
  seed
  commit_all
  git update-ref refs/remotes/origin/main HEAD
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  [[ "$output" == *'Scanning staged tree'* ]]
  assert_redacted
  git rm -q secret.txt
  git commit -qm removed
  git update-ref refs/remotes/origin/main HEAD
  run bash "$SCRIPT" --mode local
  [ "$status" -eq 0 ]
  run bash "$SCRIPT" --mode ci
  [ "$status" -ne 0 ]
  assert_redacted
}

@test "AC3 exact origin defaults and declared slash bases ignore local branches" {
  local origin_oid
  origin_oid=$(git rev-parse HEAD)
  git update-ref refs/remotes/origin/release/next HEAD
  seed
  commit_all
  git branch release/next HEAD
  git update-ref refs/heads/main "$origin_oid" HEAD
  git checkout -q --detach release/next
  run bash "$SCRIPT" --mode local --base refs/remotes/origin/release/next
  [ "$status" -ne 0 ]
  [[ "$output" == *"base ref: refs/remotes/origin/release/next"* ]]
  [[ "$output" == *"base commit: $origin_oid"* ]]
  assert_redacted
  git rm -q secret.txt
  git commit -qm removed
  git update-ref refs/remotes/origin/release/next HEAD
  git symbolic-ref refs/remotes/origin/HEAD refs/remotes/origin/release/next
  run bash "$SCRIPT" --mode local
  [ "$status" -eq 0 ]
  [[ "$output" == *"base ref: refs/remotes/origin/release/next"* ]]
  run bash "$SCRIPT" --mode local --base refs/heads/release/next
  [ "$status" -ne 0 ]
  [[ "$output" == *'fully qualified branch ref'* ]]
  run bash "$SCRIPT" --mode local --base refs/remotes/origin/main
  [ "$status" -ne 0 ]
  assert_redacted
}

@test "AC3 absent broken or invalid origin refs never fall back or fetch" {
  git config alias.fetch '!exit 99'
  git symbolic-ref --delete refs/remotes/origin/HEAD
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  [[ "$output" == *'repair refs/remotes/origin/HEAD'* ]]
  git symbolic-ref refs/remotes/origin/HEAD refs/heads/main
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  [[ "$output" == *'fully qualified branch ref'* ]]
  git symbolic-ref refs/remotes/origin/HEAD refs/remotes/origin/missing
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  [[ "$output" == *'Fetch the required origin history'* ]]
  git symbolic-ref --delete refs/remotes/origin/HEAD
  git update-ref refs/remotes/origin/HEAD HEAD
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  for ref in main refs/heads/main refs/remotes/origin/missing 'refs/remotes/origin/bad..name' refs/remotes/origin/HEAD; do
    run bash "$SCRIPT" --mode local --base "$ref"
    [ "$status" -ne 0 ]
  done
}

@test "AC3 unborn shallow and unrelated histories fail clearly" {
  git checkout -q --orphan unborn
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  [[ "$output" == *'HEAD must name a commit'* ]]
  git rm -q --cached tracked
  git commit -q --allow-empty -m unrelated
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  [[ "$output" == *'common ancestor'* ]]
  git clone -q --depth=1 "file://$BATS_TEST_TMPDIR/repo" "$BATS_TEST_TMPDIR/shallow"
  cd "$BATS_TEST_TMPDIR/shallow"
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  [[ "$output" == *'shallow history is incomplete'* ]]
}

@test "AC4 indexed bytes override clean working files and state remains unchanged" {
  seed
  git add secret.txt
  echo clean > secret.txt
  local before after
  before=$(capture_state)
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  [[ "$output" == *'Scanning staged tree'* ]]
  assert_redacted
  local working
  working=$(cat secret.txt)
  [ "$working" = clean ]
  after=$(capture_state)
  [ "$after" = "$before" ]
}

@test "AC4 partial staging in both directions excludes untracked and deleted bytes" {
  seed
  local secret_line
  secret_line=$(cat secret.txt)
  printf '%s\nclean\n' "$secret_line" > secret.txt
  git add secret.txt
  printf 'clean\n%s\n' "$secret_line" > secret.txt
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  assert_redacted
  echo clean > secret.txt
  git add secret.txt
  printf 'clean\n%s\n' "$secret_line" > secret.txt
  seed untracked.txt
  run bash "$SCRIPT" --mode local
  [ "$status" -eq 0 ]
  git rm -qf --cached secret.txt
  run bash "$SCRIPT" --mode local
  [ "$status" -eq 0 ]
}

@test "AC4 unusual indexed paths keep relative finding locations and exact-path policy" {
  local path=$'odd path\tname.txt'
  seed "$path"
  git add -- "$path"
  local before after
  before=$(capture_state)
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  [[ "$output" != *"$SECRET"* ]]
  [[ "$output" == *github-pat* ]]
  after=$(capture_state)
  [ "$after" = "$before" ]
}

@test "AC4 unmerged index fails snapshot creation without changing state" {
  git checkout -qb side
  echo side > tracked
  commit_all
  git checkout -q main
  echo main > tracked
  commit_all
  run git merge side -m conflict
  [ "$status" -ne 0 ]
  local before after
  before=$(capture_state)
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  [[ "$output" == *'cannot capture the full index tree'* ]]
  after=$(capture_state)
  [ "$after" = "$before" ]
}

@test "AC5 empty ranges staged diffs trees and histories without patch content pass" {
  run bash "$SCRIPT" --mode local
  [ "$status" -eq 0 ]
  [[ "$output" == *'no content to scan'* ]]
  git rm -q tracked
  git commit -qm empty
  git update-ref refs/remotes/origin/main HEAD
  run bash "$SCRIPT" --mode local
  [ "$status" -eq 0 ]
  git commit -q --allow-empty -m empty
  run bash "$SCRIPT" --mode local
  [ "$status" -eq 0 ]
  git checkout -q --orphan empty
  git commit -q --allow-empty -m root
  git update-ref refs/remotes/origin/main HEAD
  for mode in local ci; do
    run bash "$SCRIPT" --mode "$mode"
    [ "$status" -eq 0 ]
  done
}

stub_scanner() {
  mkdir -p "$BATS_TEST_TMPDIR/bin"
  printf '#!/usr/bin/env bash\n%s\n' "$1" > "$BATS_TEST_TMPDIR/bin/gitleaks"
  chmod +x "$BATS_TEST_TMPDIR/bin/gitleaks"
  export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
}

@test "AC5 scanner exits diagnostics and unexpected zero commits all fail" {
  for body in 'exit 7' 'echo "ERR git failed"' 'echo "fatal: git failed"' 'echo "0 commits scanned"' $'printf "\\033[31mERR\\033[0m git failed\\n1 commits scanned\\n"'; do
    stub_scanner "$body"
    for mode in local ci; do
      run bash "$SCRIPT" --mode "$mode"
      [ "$status" -ne 0 ]
    done
  done
  stub_scanner 'echo "ERR git failed"'
  # Even a valid empty history range cannot excuse scanner errors.
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  [[ "$output" == *'scanner reported an error in branch history'* ]]
}

@test "AC5 Git read and snapshot errors block independently of scanner success" {
  local real_git
  real_git=$(command -v git)
  export REAL_GIT="$real_git"
  mkdir "$BATS_TEST_TMPDIR/bin"
  cat > "$BATS_TEST_TMPDIR/bin/git" <<'STUB'
#!/usr/bin/env bash
for arg in "$@"; do
  [ "$arg" != "$FAIL_GIT_COMMAND" ] || exit 9
done
[ "$1" != fetch ] || exit 99
exec "$REAL_GIT" "$@"
STUB
  chmod +x "$BATS_TEST_TMPDIR/bin/git"
  export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
  local before after
  before=$(capture_state)
  for command in log write-tree commit-tree; do
    export FAIL_GIT_COMMAND="$command"
    run bash "$SCRIPT" --mode local
    [ "$status" -ne 0 ]
    after=$(capture_state)
    [ "$after" = "$before" ]
  done
  export FAIL_GIT_COMMAND=fetch
  run bash "$SCRIPT" --mode local
  [ "$status" -eq 0 ]
}

@test "AC6 full-fetch clones scan unrelated branches on main and PR merge checkouts" {
  git checkout -qb seeded
  seed
  commit_all
  git checkout -q main
  git checkout -qb feature
  echo feature > feature
  commit_all
  git checkout -q main
  git merge -q --no-ff feature -m merge
  git update-ref refs/pull/1/merge HEAD
  git reset -q --hard refs/remotes/origin/main
  git clone -q --no-local "$BATS_TEST_TMPDIR/repo" "$BATS_TEST_TMPDIR/clone"
  cd "$BATS_TEST_TMPDIR/clone"
  git fetch -q origin '+refs/pull/*:refs/pull/*'
  for ref in main refs/pull/1/merge origin/seeded; do
    git checkout -q --detach "$ref"
    local shallow
    shallow=$(git rev-parse --is-shallow-repository)
    [ "$shallow" = false ]
    run bash "$SCRIPT" --mode ci
    [ "$status" -ne 0 ]
    assert_redacted
  done
}

@test "AC8 allow markers work with or without justification in both modes" {
  local reason
  for reason in '' ' synthetic test value'; do
    seed
    printf ' # gitleaks:allow%s\n' "$reason" >> secret.txt
    # Put the marker on the same line as the generated value.
    tr -d '\n' < secret.txt > marked
    mv marked secret.txt
    commit_all
    for mode in local ci; do
      run bash "$SCRIPT" --mode "$mode"
      [ "$status" -eq 0 ]
      [[ "$output" != *"$SECRET"* ]]
    done
  done
}

@test "AC4 deleting a base secret from the index excludes its working bytes" {
  seed
  commit_all
  git update-ref refs/remotes/origin/main HEAD
  git rm -q --cached secret.txt
  local before after
  before=$(capture_state)
  run bash "$SCRIPT" --mode local
  [ "$status" -eq 0 ]
  after=$(capture_state)
  [ "$after" = "$before" ]
}

@test "AC5 snapshot scan errors and false zero counts block after clean history" {
  stub_scanner 'case "$*" in *..*) echo "0 commits scanned" ;; *) echo "1 commits scanned"; echo "ERR snapshot scan failed" ;; esac'
  local before after
  before=$(capture_state)
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  [[ "$output" == *'scanner reported an error in staged tree'* ]]
  after=$(capture_state)
  [ "$after" = "$before" ]
  stub_scanner 'echo "0 commits scanned"'
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  [[ "$output" == *'expected patch content in staged tree'* ]]
}

@test "AC8 local snapshots honor the exact repository-relative fixture exception" {
  local config="${BATS_TEST_DIRNAME}/../../.devtools/config/gitleaks.toml"
  sed -E 's/^# (\[\[allowlists\]\]|description =|paths =)/\1/' "$config" > .gitleaks.toml
  mkdir -p tests/fixtures
  openssl genrsa 1024 > tests/fixtures/private-key.pem 2>/dev/null
  git add -- .gitleaks.toml tests/fixtures/private-key.pem
  run bash "$SCRIPT" --mode local
  [ "$status" -eq 0 ]
  seed tests/fixtures/private-key.pem.bak
  git add -- tests/fixtures/private-key.pem.bak
  run bash "$SCRIPT" --mode local
  [ "$status" -ne 0 ]
  assert_redacted tests/fixtures/private-key.pem.bak
}
