#!/usr/bin/env bash
# Compatibility entry point during the N-2 transition.
set -uo pipefail

if command -v agentrepo >/dev/null 2>&1 && help=$(agentrepo checks diagnose --help 2>/dev/null); then
  case $help in
  *'agentrepo checks diagnose [pull-request-number]'*) exec agentrepo checks diagnose "$@" ;;
  esac
fi

script_dir=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
exec bash "$script_dir/diagnose-legacy.sh" "$@"
