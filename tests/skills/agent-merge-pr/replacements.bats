#!/usr/bin/env bats
# cspell:words gosec lintfast noqa nolint
setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_setup
}

# The fixture policy covers lint.yaml only, so these tests add mise.lock and hooks.toml.
inspect_lock() {
  jq -n '{golden_paths:"^golden/",loosening_markers:"noqa|nolint|vale (off|disable)",gate_config_paths:"(^|/)(lint[.]yaml|mise[.]lock|hooks[.]toml)$",owner_paths:"^owned/",disposable_ignored:"^cache/"}' |
    uv run --quiet --script "$POLICY" inspect "$BASE_OID" "$HEAD_OID"
}

# Write each "path=content" pair into one commit; content may hold newlines.
snapshot() {
  local pair path
  for pair in "$@"; do
    path=${pair%%=*}
    mkdir -p "$(dirname "$path")"
    printf '%s\n' "${pair#*=}" >"$path"
    git add -- "$path"
  done
  git commit --quiet -m 'test: snapshot'
  HEAD_OID=$(git rev-parse HEAD)
}

# Treat the current commit as the pinned base for the next snapshot.
rebase_onto_head() { BASE_OID=$HEAD_OID; }

reasons() { jq -r '.findings[] | join(": ")' <<<"$output"; }

lock_old=$'[tools]\nchecksum = "sha256:aaa"\nchecksum = "sha256:bbb"'
lock_new=$'[tools]\nchecksum = "sha256:ccc"\nchecksum = "sha256:ddd"'

@test "AC1 mise.lock checksum replacements are not a removed gate" {
  snapshot "mise.lock=$lock_old"
  rebase_onto_head
  snapshot "mise.lock=$lock_new"
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(jq '.findings|length' <<<"$output")" -eq 0 ]
}

@test "AC1 nested mise.lock replacements and surplus additions are not a removed gate" {
  snapshot "sub/mise.lock=$lock_old"
  rebase_onto_head
  snapshot "sub/mise.lock=${lock_new}"$'\nchecksum = "sha256:eee"'
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(jq '.findings|length' <<<"$output")" -eq 0 ]
}

@test "AC1 a real gate removal beside replaced checksums is the only finding" {
  snapshot "mise.lock=$lock_old"$'\nlint = "old"'
  rebase_onto_head
  snapshot "mise.lock=$lock_new"
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(jq '.findings|length' <<<"$output")" -eq 1 ]
  [ "$(reasons)" = "mise.lock: removes a hook or gate" ]
}

@test "AC1 other findings in the same mise.lock diff remain" {
  snapshot "mise.lock=$lock_old"
  rebase_onto_head
  snapshot "mise.lock=${lock_new}"$'\nvalue = 1 # nolint'
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(reasons)" = "mise.lock: adds a suppression or exclusion" ]
}

@test "AC2 deleting checksums or removing more than were added still flags" {
  snapshot "mise.lock=$lock_old"
  rebase_onto_head
  snapshot "mise.lock=[tools]"
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(jq '.findings|length' <<<"$output")" -eq 2 ]

  git reset --quiet --hard "$BASE_OID"
  snapshot "mise.lock=[tools]"$'\nchecksum = "sha256:ccc"'
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(jq '.findings|length' <<<"$output")" -eq 2 ]
  [ "$(reasons | sort -u)" = "mise.lock: removes a hook or gate" ]
}

@test "AC2 a checksum replaced outside mise.lock still flags" {
  snapshot 'hooks.toml=checksum = "sha256:aaa"'
  rebase_onto_head
  snapshot 'hooks.toml=checksum = "sha256:bbb"'
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(reasons)" = "hooks.toml: removes a hook or gate" ]
}

unicode_checksum_note() {
  snapshot 'mise.lock=checksum = "sha256:aaa"'
  rebase_onto_head
  snapshot "mise.lock=note = 'padding${1}+checksum = \"sha256:bbb\"'"
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(reasons)" = "mise.lock: removes a hook or gate" ]
}

@test "AC2 U+0085 inside an added value preserves the checksum deletion finding" {
  unicode_checksum_note $'\xc2\x85'
}

@test "AC2 U+2028 inside an added value preserves the checksum deletion finding" {
  unicode_checksum_note $'\xe2\x80\xa8'
}

@test "AC2 U+2029 inside an added value preserves the checksum deletion finding" {
  unicode_checksum_note $'\xe2\x80\xa9'
}

@test "AC2 CR inside an added value preserves the checksum deletion finding" {
  unicode_checksum_note $'\r'
}

@test "AC2 each real gate line still flags when removed" {
  local line file
  for file in lint.yaml mise.lock hooks.toml; do
    for line in 'lint = "x"' 'lintfast = "x"' 'check-foo = "x"' '- id: foo' 'mise run test'; do
      git reset --quiet --hard "$HEAD_OID"
      snapshot "$file=keep"$'\n'"$line"
      rebase_onto_head
      snapshot "$file=keep"
      run inspect_lock
      [ "$status" -eq 0 ]
      [ "$(reasons)" = "$file: removes a hook or gate" ] || {
        echo "not flagged: $file / $line: $output"
        return 1
      }
    done
  done
}

@test "AC2 a gate line removed beside matching checksum replacements in mise.lock still flags" {
  local line
  for line in 'lint = "x"' 'lintfast = "x"' 'check-foo = "x"' '- id: foo' 'mise run test'; do
    git reset --quiet --hard "$HEAD_OID"
    snapshot "mise.lock=$lock_old"$'\n'"$line"
    rebase_onto_head
    snapshot "mise.lock=$lock_new"
    run inspect_lock
    [ "$status" -eq 0 ]
    [ "$(reasons | sort -u)" = "mise.lock: removes a hook or gate" ] || {
      echo "not flagged: $line: $output"
      return 1
    }
  done
}

nolint_base() {
  snapshot 'vendor/modules.txt=# a v1' $'vendor/dep/x.go=package dep\nvar a = 1'
  rebase_onto_head
}

@test "AC3 inline nolint in a re-vendored Go file is not a suppression finding" {
  nolint_base
  snapshot 'vendor/modules.txt=# a v2' $'vendor/dep/x.go=package dep\nvar a = 1 //nolint:gosec'
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(jq '.findings|length' <<<"$output")" -eq 0 ]
}

@test "AC3 another marker on the same line or another line in the diff still flags" {
  nolint_base
  snapshot 'vendor/modules.txt=# a v2' $'vendor/dep/x.go=package dep\nvar a = 1 //nolint noqa'
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(reasons)" = "vendor/dep/x.go: adds a suppression or exclusion" ]

  git reset --quiet --hard "$BASE_OID"
  snapshot 'vendor/modules.txt=# a v2' $'vendor/dep/x.go=package dep\nvar a = 1 //nolint\nvar b = 2 // noqa'
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(jq '.findings|length' <<<"$output")" -eq 1 ]

  git reset --quiet --hard "$BASE_OID"
  snapshot 'vendor/modules.txt=# a v2' $'vendor/dep/x.go=package dep\nvar a = 1 //nolint' 'first/party.go=var c = 3 //nolint'
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(reasons)" = "first/party.go: adds a suppression or exclusion" ]
}

@test "AC4 a non-nolint marker in a re-vendored Go file still flags" {
  nolint_base
  snapshot 'vendor/modules.txt=# a v2' $'vendor/dep/x.go=package dep\nvar a = 1 // noqa'
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(reasons)" = "vendor/dep/x.go: adds a suppression or exclusion" ]
}

@test "AC4 inline nolint in first-party Go code still flags" {
  snapshot 'vendor/modules.txt=# a v1' 'src/a.go=package a'
  rebase_onto_head
  snapshot 'vendor/modules.txt=# a v2' $'src/a.go=package a\nvar a = 1 //nolint:gosec'
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(reasons)" = "src/a.go: adds a suppression or exclusion" ]
}

@test "AC4 vendored nolint still flags when modules.txt is unchanged" {
  nolint_base
  snapshot $'vendor/dep/x.go=package dep\nvar a = 1 //nolint:gosec'
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(reasons)" = "vendor/dep/x.go: adds a suppression or exclusion" ]
}

@test "AC4 vendored nolint still flags when modules.txt is absent at the base" {
  snapshot $'vendor/dep/x.go=package dep\nvar a = 1'
  rebase_onto_head
  snapshot 'vendor/modules.txt=# a v1' $'vendor/dep/x.go=package dep\nvar a = 1 //nolint:gosec'
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(reasons)" = "vendor/dep/x.go: adds a suppression or exclusion" ]
}

@test "AC4 vendored nolint still flags in a non-Go file" {
  snapshot 'vendor/modules.txt=# a v1' 'vendor/dep/x.py=a = 1'
  rebase_onto_head
  snapshot 'vendor/modules.txt=# a v2' $'vendor/dep/x.py=a = 1  # nolint'
  run inspect_lock
  [ "$status" -eq 0 ]
  [ "$(reasons)" = "vendor/dep/x.py: adds a suppression or exclusion" ]
}
