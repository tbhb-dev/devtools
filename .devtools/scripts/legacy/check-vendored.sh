#!/usr/bin/env bash
set -euo pipefail
env -u GIT_DIR -u GIT_COMMON_DIR -u GIT_WORK_TREE -u GIT_INDEX_FILE \
  -u GIT_OBJECT_DIRECTORY vendir sync
rc=0
git diff --stat --exit-code -- vendir.yml vendir.lock.yml .config/mise/conf.d .devtools || rc=1
untracked=$(git ls-files --others --exclude-per-directory=.gitignore -- .config/mise/conf.d .devtools)
if [ -n "$untracked" ]; then
  # shellcheck disable=SC2086 # Preserve the frozen gate's word splitting and globbing.
  printf '?? %s\n' $untracked
  rc=1
fi
if [ "$rc" -ne 0 ]; then
  echo "vendored payload drifted from what git holds -- review the resync above, then stage and commit it or fix vendir.yml" >&2
  exit 1
fi
echo "vendored payload matches what git holds"
