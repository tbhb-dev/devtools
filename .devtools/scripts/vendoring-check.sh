#!/usr/bin/env bash
# N-2 consumers keep the frozen gate until their selected binary supports it.
set -euo pipefail
command=$1
shift
binary=$(mise which agentrepo 2>/dev/null) || binary=''
if [ -z "$binary" ] || [ ! -x "$binary" ]; then
  binary=$(command -v agentrepo) || binary=''
fi
if [ -n "$binary" ] && "$binary" "$command" --help >/dev/null 2>&1; then
  exec "$binary" "$command" "$@"
fi
exec bash "${DEVTOOLS_DIR:-.devtools}/scripts/legacy/$command.sh" "$@"
