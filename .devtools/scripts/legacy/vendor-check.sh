#!/usr/bin/env bash
set -euo pipefail
tracked=$(git ls-files -- vendor/modules.txt)
if [ -z "$tracked" ]; then
  echo "vendor-check: skipping module without tracked vendor/modules.txt"
  exit 0
fi
go mod tidy
go mod vendor
rc=0
git diff --exit-code -- go.mod go.sum vendor/ || rc=1
untracked=$(git ls-files --others --exclude-per-directory=.gitignore -- go.mod go.sum vendor/)
if [ -n "$untracked" ]; then
  printf '%s\n' "$untracked"
  rc=1
fi
if [ "$rc" -ne 0 ]; then
  echo "vendor drift detected — run 'mise run devtools:vendor' and commit" >&2
  exit 1
fi
