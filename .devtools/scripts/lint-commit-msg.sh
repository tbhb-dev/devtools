#!/usr/bin/env bash
# Preview the commit-msg gates without changing the author's draft.
set -euo pipefail

main() {
  local identity trailers
  preview_dir=$(mktemp -d "${TMPDIR:-/tmp}/commit-preview.XXXXXX")
  trap 'rm -rf -- "$preview_dir"' EXIT
  preview="$preview_dir/COMMIT_EDITMSG"
  cp -- COMMIT_AGENTMSG "$preview"
  trailers=$(git interpret-trailers --parse "$preview")
  if ! grep -qi '^Signed-off-by:' <<<"$trailers"; then
    identity=$(git var GIT_COMMITTER_IDENT)
    identity=${identity% *}
    identity=${identity% *}
    git -c trailer.separators=: interpret-trailers --in-place \
      --trailer "Signed-off-by: $identity" "$preview"
  fi
  prek run --stage commit-msg --commit-msg-filename "$preview"
}

main "$@"
