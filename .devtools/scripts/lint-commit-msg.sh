#!/usr/bin/env bash
# Probe the consumer-selected binary; operational failures must not rerun gates.
if agentrepo commit-preview --probe >/dev/null 2>&1; then
  exec agentrepo commit-preview "$@"
fi
# Frozen fallback stays available throughout the N-2 compatibility window.
exec bash "$(dirname "${BASH_SOURCE[0]}")/lint-commit-msg-shell.sh" "$@"
