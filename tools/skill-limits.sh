#!/usr/bin/env bash
# Compatibility entry point for the offline skill-limit gate.
set -euo pipefail
export LC_ALL=C
unset CDPATH GREP_OPTIONS
IFS=$' \t\n'

if command -v agentrepo >/dev/null 2>&1; then
  # Root help is read-only even in releases predating this capability.
  # A failed probe cannot establish absence: preserve its failure.
  help=$(agentrepo --help) || exit "$?"
  if grep -Eq '^  check-skill-limits[[:space:]]' <<<"$help"; then
    exec agentrepo check-skill-limits -- "$@"
  fi
fi
# Fallback retirement: https://github.com/tbhb-dev/devtools/issues/847
exec bash "$(dirname "${BASH_SOURCE[0]}")/skill-limits-legacy.sh" "$@"
