#!/usr/bin/env bash
set -eu
if [[ -f .claude/settings.json ]]; then
  jq 'del(.hooks)' .claude/settings.json >.claude/settings.json.tmp
  mv .claude/settings.json.tmp .claude/settings.json
  [[ $(jq 'length' .claude/settings.json) -gt 0 ]] || rm -f .claude/settings.json
fi
if [[ -f .agents/hooks.json ]]; then
  jq 'del(.apm)' .agents/hooks.json >.agents/hooks.json.tmp
  mv .agents/hooks.json.tmp .agents/hooks.json
  [[ $(jq 'length' .agents/hooks.json) -gt 0 ]] || rm -f .agents/hooks.json
fi
if [[ -f .codex/hooks.json ]]; then
  jq 'del(.hooks)' .codex/hooks.json >.codex/hooks.json.tmp
  mv .codex/hooks.json.tmp .codex/hooks.json
  [[ $(jq 'length' .codex/hooks.json) -gt 0 ]] || rm -f .codex/hooks.json
fi
rm -f .claude/apm-hooks.json .agents/apm-hooks.json .codex/apm-hooks.json
apm install
