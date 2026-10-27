#!/usr/bin/env bash
# Compatibility entry point: older binaries retain the frozen implementation.
set -euo pipefail
root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
if command -v agentrepo >/dev/null 2>&1; then
  help=$(agentrepo --help) || exit $?
  if [[ $help == *'  skill-tokens '* ]]; then
    exec agentrepo skill-tokens "$@"
  fi
fi
# Retirement after the compatibility window: https://github.com/tbhb-dev/devtools/issues/846
exec bash "$root/legacy/skill-tokens.sh" "$@"
