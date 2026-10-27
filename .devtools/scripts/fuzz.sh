#!/usr/bin/env bash
# Keep the frozen fallback throughout the N-2 compatibility window.
set -euo pipefail

binary=$(mise which agentrepo 2>/dev/null) || binary=''
if [ -z "$binary" ] || [ ! -x "$binary" ]; then
  binary=$(command -v agentrepo) || binary=''
fi
if [ -n "$binary" ] && "$binary" fuzz-go --supported >/dev/null 2>&1; then
  exec "$binary" fuzz-go "$@"
fi
exec bash "$(dirname "$0")/legacy/fuzz.sh" "$@"
