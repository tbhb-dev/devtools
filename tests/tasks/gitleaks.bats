#!/usr/bin/env bats

setup() {
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
