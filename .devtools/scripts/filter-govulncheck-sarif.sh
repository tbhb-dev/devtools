#!/usr/bin/env bash
# Keep call-graph findings for code scanning; never overwrite the raw input.
set -euo pipefail
if [[ "$1" == "$2" ]]; then
  echo 'Input and output SARIF paths must differ.' >&2
  exit 1
fi
jq '(.runs[] | select(has("results")) | .results) |= map(select(.codeFlows | length > 0))' "$1" >"$2"
