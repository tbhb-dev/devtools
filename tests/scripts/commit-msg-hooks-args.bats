#!/usr/bin/env bats
#
# Tests for the message-file argument of scripts/cspell-commit-msg.sh and
# scripts/vale-commit-msg.sh. A hook run without the argument must name the
# missing path rather than die on an unbound variable.
#
# The linters are stubs on PATH: the contract under test is the argument
# handling, not what cspell or vale say about the text.

setup() {
  SCRIPTS="${BATS_TEST_DIRNAME}/../../scripts"
  STUB_DIR="${BATS_TEST_TMPDIR}/stub"
  mkdir -p "$STUB_DIR"
  for tool in cspell vale; do
    printf '#!/usr/bin/env bash\ncat >/dev/null\nexit 0\n' >"${STUB_DIR}/${tool}"
    chmod +x "${STUB_DIR}/${tool}"
  done
  PATH="${STUB_DIR}:${PATH}"
  MSG="${BATS_TEST_TMPDIR}/MSG"
  printf 'fix: a message\n' >"$MSG"
}

@test "cspell-commit-msg without an argument names the required path" {
  run bash "${SCRIPTS}/cspell-commit-msg.sh"
  [ "$status" -ne 0 ]
  [ "${#lines[@]}" -eq 1 ]
  [[ "$output" == *"commit-message file path is required"* ]]
  [[ "$output" != *"unbound variable"* ]]
}

@test "cspell-commit-msg with an argument succeeds" {
  run bash "${SCRIPTS}/cspell-commit-msg.sh" "$MSG"
  [ "$status" -eq 0 ]
}

@test "vale-commit-msg without an argument names the required path" {
  run bash "${SCRIPTS}/vale-commit-msg.sh"
  [ "$status" -ne 0 ]
  [ "${#lines[@]}" -eq 1 ]
  [[ "$output" == *"commit-message file path is required"* ]]
  [[ "$output" != *"unbound variable"* ]]
}

@test "vale-commit-msg with an argument succeeds" {
  run bash "${SCRIPTS}/vale-commit-msg.sh" "$MSG"
  [ "$status" -eq 0 ]
}
