#!/usr/bin/env bash
if [ "$#" -eq 0 ] || { [ "$#" -eq 1 ] && [ "$1" = --staged ]; }; then
  printf 'usage: marker-scan.sh [--staged] <path> [path ...]\n' >&2
  exit 2
fi
output=$(agentrepo markers scan "$@")
status=$?
[ -z "$output" ] || printf '%s\n' "$output"
[ "$status" -eq 0 ] || { [ "$status" -eq 1 ] && [ -n "$output" ]; } || printf 'marker-scan.sh: agentrepo markers scan failed (exit %s)\n' "$status"
exit "$status"
