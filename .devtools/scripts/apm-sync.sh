#!/usr/bin/env bash
set -eu
# Resolve the consumer-selected version before PATH. Probe before any mutation.
binary=$(mise which agentrepo 2>/dev/null) || binary=''
if [[ -z $binary || ! -x $binary ]]; then
  binary=$(command -v agentrepo) || binary=''
fi
if [[ -n $binary ]] && "$binary" apm-sync --help >/dev/null 2>&1; then
  exec "$binary" apm-sync "$@"
fi
# Frozen N-2 fallback for releases without the reconciliation capability.
if [[ -f .claude/settings.json ]]; then
  jq 'del(.hooks)' .claude/settings.json >.claude/settings.json.tmp
  mv .claude/settings.json.tmp .claude/settings.json
  length=$(jq 'length' .claude/settings.json)
  [[ $length -gt 0 ]] || rm -f .claude/settings.json
fi
if [[ -f .agents/hooks.json ]]; then
  jq 'del(.apm)' .agents/hooks.json >.agents/hooks.json.tmp
  mv .agents/hooks.json.tmp .agents/hooks.json
  length=$(jq 'length' .agents/hooks.json)
  [[ $length -gt 0 ]] || rm -f .agents/hooks.json
fi
if [[ -f .codex/hooks.json ]]; then
  jq 'del(.hooks)' .codex/hooks.json >.codex/hooks.json.tmp
  mv .codex/hooks.json.tmp .codex/hooks.json
  length=$(jq 'length' .codex/hooks.json)
  [[ $length -gt 0 ]] || rm -f .codex/hooks.json
fi
rm -f .claude/apm-hooks.json .agents/apm-hooks.json .codex/apm-hooks.json
apm install
