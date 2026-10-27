#!/usr/bin/env bash
# Prefer the consumer-selected capability; retain the N-2 shell contract.
set -euo pipefail
export LC_ALL=C
unset CDPATH GREP_OPTIONS

binary=$(mise which agentrepo 2>/dev/null) || binary=''
if [ -z "$binary" ]; then
  binary=$(command -v agentrepo) || binary=''
fi
if [ -n "$binary" ] && "$binary" go-ldflags --help >/dev/null 2>&1; then
  exec "$binary" go-ldflags "$@"
fi
exec bash "$(dirname "$0")/legacy/go-ldflags.sh" "$@"
