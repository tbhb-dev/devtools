#!/usr/bin/env bash
# Capability probing must never generate output or repeat a failed generation.
set -euo pipefail
if command -v agentrepo >/dev/null 2>&1; then
  status=0
  capability=$(agentrepo generate-skills --capability 2>&1) || status=$?
  if [ "$status" -eq 0 ] && [ "$capability" = generate-skills-v1 ]; then
    exec agentrepo generate-skills "$@"
  fi
  case "$status:$capability" in
  '1:unknown command "generate-skills" for "agentrepo"' | '5:unknown command "generate-skills" for "agentrepo"') ;;
  *)
    printf 'generate-skills: capability probe failed (exit %s): %s\n' "$status" "$capability" >&2
    exit 1
    ;;
  esac
fi
# Keep the frozen fallback throughout N-2 compatibility support (#825).
exec bash "$(dirname "${BASH_SOURCE[0]}")/compat/generate-skills.sh" "$@"
