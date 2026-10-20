#!/usr/bin/env bash
# Preview the commit-msg gates without changing the author's draft.
set -euo pipefail

conf_value() {
  local file=$1 key=$2
  [ -f "$file" ] || return 1
  awk -v key="$key" '
    /^[ \t]*#/ || !/=/ { next }
    {
      k = $0
      sub(/=.*/, "", k)
      gsub(/^[ \t]+|[ \t]+$/, "", k)
      if (k != key) next
      v = $0
      sub(/^[^=]*=/, "", v)
      gsub(/^[ \t]+|[ \t]+$/, "", v)
      print v
      found = 1
      exit
    }
    END { exit found ? 0 : 1 }
  ' "$file"
}

configured_signoff() {
  local common config_dir repo value
  if [ -n "${DEVTOOLS_SIGNOFF_IDENTITY+set}" ]; then
    printf '%s\n' "$DEVTOOLS_SIGNOFF_IDENTITY"
    return 0
  fi
  common=$(git rev-parse --path-format=absolute --git-common-dir) || return $?
  repo=$(basename "$(dirname "$common")") || return $?
  config_dir=${XDG_CONFIG_HOME:-$HOME/.config}/devtools
  if value=$(conf_value "$config_dir/$repo.conf" signoff_identity) ||
    value=$(conf_value "$config_dir/devtools.conf" signoff_identity); then
    printf '%s\n' "$value"
    return 0
  fi
}

signoff_identity() {
  local configured identity name email status
  configured=$(configured_signoff || true)
  if [ -n "$configured" ]; then
    identity=$configured
  else
    name=$(git config --get user.name || printf 'UNSET\n')
    email=$(git config --get user.email || printf 'UNSET\n')
    identity="$name <$email>"
  fi

  if commit-lint --check-signoff "$identity"; then
    printf '%s\n' "$identity"
    return 0
  else
    status=$?
  fi
  if [ -z "$configured" ] && [ "$status" -eq 1 ]; then
    printf 'set signoff_identity in ~/.config/devtools/<repository>.conf or ~/.config/devtools/devtools.conf\n' >&2
  fi
  return "$status"
}

main() {
  local identity trailers
  preview_dir=$(mktemp -d "${TMPDIR:-/tmp}/commit-preview.XXXXXX")
  trap 'rm -rf -- "$preview_dir"' EXIT
  preview="$preview_dir/COMMIT_EDITMSG"
  cp -- COMMIT_AGENTMSG "$preview"
  trailers=$(git interpret-trailers --parse "$preview")
  if ! grep -qi '^Signed-off-by:' <<<"$trailers"; then
    identity=$(signoff_identity) || return $?
    git -c trailer.separators=: interpret-trailers --in-place \
      --trailer "Signed-off-by: $identity" "$preview"
  fi
  prek run --stage commit-msg --commit-msg-filename "$preview"
}

main "$@"
