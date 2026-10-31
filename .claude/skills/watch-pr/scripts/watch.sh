#!/usr/bin/env bash
# Compatibility entry point during the N-2 transition.
set -uo pipefail

if help=$(agentrepo checks watch --help 2>/dev/null); then
  case $help in
  *'agentrepo checks watch [pull-request-number]'*) exec agentrepo checks watch "$@" ;;
  esac
fi

exec bash "${BASH_SOURCE[0]%/*}/watch-checks.sh" "$@"
