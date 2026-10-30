#!/usr/bin/env bash
# Keep the frozen fallback until #333 and #544 land and the N-2 window passes.
if probe=$(agentrepo checks watch --help 2>/dev/null); then
  case "$probe" in
  *'agentrepo checks watch [pull-request-number]'*) exec agentrepo checks watch "$@" ;;
  esac
fi
exec bash "$(dirname "${BASH_SOURCE[0]}")/watch-checks-legacy.sh" "$@"
