#!/usr/bin/env bash
# Compatibility entry point; probe without reading or changing the repository.
set -euo pipefail
export LC_ALL=C
if command -v agentrepo >/dev/null 2>&1; then
  status=0
  capability=$(agentrepo check-script-hygiene --probe 2>&1) || status=$?
  if [ "$status" -eq 0 ] && [ "$capability" = script-hygiene-v1 ]; then
    exec agentrepo check-script-hygiene "$@"
  fi
  if [ "$status" -ne 5 ] || [ "$capability" != 'unknown command "check-script-hygiene" for "agentrepo"' ]; then
    printf 'check-script-hygiene: capability probe failed (status %s): %s\n' "$status" "$capability" >&2
    exit 1
  fi
fi
# Frozen fallback stays until #833 establishes N-2 retirement eligibility.
exec bash "$(dirname "$0")/legacy/check-script-hygiene.sh" "$@"
