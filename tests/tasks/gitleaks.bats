#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  local task="${BATS_TEST_DIRNAME}/../../.devtools/tasks/security.toml"
  SCRIPT="${BATS_TEST_TMPDIR}/gitleaks.sh"
  # Run the shipped task body, including its exit-status and commit guard.
  sed -n "/^run = '''$/,/^'''$/p" "$task" | sed '1d;$d' > "$SCRIPT"
  mkdir "${BATS_TEST_TMPDIR}/repo"
  cd "${BATS_TEST_TMPDIR}/repo"
  unset GITLEAKS_CONFIG GITLEAKS_CONFIG_TOML
  export NO_COLOR=1
}

@test "a committed fake token reports its rule and location without the secret" {
  git init -q
  local fake_token
  fake_token="ghp_$(openssl rand -hex 18)"
  printf 'fake_token=%s\n' "$fake_token" > fake-token.txt
  git add -- fake-token.txt
  # Fixture commits must not inherit signing or hooks from the host.
  git -c core.hooksPath=/dev/null -c commit.gpgSign=false \
    -c user.name=Test -c user.email=test@example.com commit -qm fixture

  run bash "$SCRIPT"

  [ "$status" -eq 1 ]
  if printf '%s\n' "$output" | grep -Fq -- "$fake_token"; then
    echo "task output exposed the generated token" >&2
    return 1
  fi
  [[ $output == *"github-pat"* ]]
  [[ $output == *"fake-token.txt"* ]]
  [[ $output =~ Line:[[:space:]]+1 ]]
  [[ $output == *"leaks found"* ]]
}

@test "an unresolvable repository fails the zero-commits guard" {
  run bash "$SCRIPT"

  [ "$status" -eq 1 ]
  [[ $output == *"gitleaks walked 0 commits"* ]]
}

install_fixture_policy() {
  local config="${BATS_TEST_DIRNAME}/../../.devtools/config/gitleaks.toml"
  # Activate the documented example exactly as a consumer would.
  sed -E 's/^# (\[\[allowlists\]\]|description =|paths =)/\1/' "$config" > .gitleaks.toml
  git init -q
}

commit_fixture_policy() {
  git add -- .gitleaks.toml tests
  git -c core.hooksPath=/dev/null -c commit.gpgSign=false \
    -c user.name=Test -c user.email=test@example.com commit -qm fixture
}

@test "the shipped template keeps private-key detection enabled by default" {
  cp "${BATS_TEST_DIRNAME}/../../.devtools/config/gitleaks.toml" .gitleaks.toml
  git init -q
  mkdir -p tests/fixtures
  openssl genrsa 1024 \
    > tests/fixtures/private-key.pem 2>/dev/null
  commit_fixture_policy

  run bash "$SCRIPT"

  [ "$status" -eq 1 ]
  [[ $output == *"private-key"* ]]
  [[ $output == *"tests/fixtures/private-key.pem"* ]]
}

@test "the documented exact fixture exception permits a synthetic private key" {
  install_fixture_policy
  mkdir -p tests/fixtures
  openssl genrsa 1024 \
    > tests/fixtures/private-key.pem 2>/dev/null
  commit_fixture_policy

  run bash "$SCRIPT"

  [ "$status" -eq 0 ]
  [[ $output == *"no leaks found"* ]]
}

@test "fixture policy retains default rules outside the exact allowed path" {
  install_fixture_policy
  local path
  for path in tests/fixtures/other.pem tests/fixtures/private-key/pem \
    tests/fixtures/private-key.pem.bak nested/tests/fixtures/private-key.pem; do
    mkdir -p "$(dirname "$path")"
    printf 'token=ghp_%s\n' "$(openssl rand -hex 18)" > "$path"
  done
  mkdir -p tests
  printf 'token=ghp_%s\n' "$(openssl rand -hex 18)" > tests/production.txt
  commit_fixture_policy
  # Include a path with a prefix to exercise the start anchor too.
  git add -- nested
  git -c core.hooksPath=/dev/null -c commit.gpgSign=false \
    -c user.name=Test -c user.email=test@example.com commit -qm prefix

  run bash "$SCRIPT"

  [ "$status" -eq 1 ]
  [[ $output == *"github-pat"* ]]
  for path in tests/production.txt tests/fixtures/other.pem \
    tests/fixtures/private-key/pem tests/fixtures/private-key.pem.bak \
    nested/tests/fixtures/private-key.pem; do
    [[ $output == *"$path"* ]]
  done
}
