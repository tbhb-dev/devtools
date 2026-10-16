#!/usr/bin/env bash
# Compatibility entry point for the shared payload script.
set -euo pipefail
exec bash "${DEVTOOLS_DIR:-$(dirname "$0")/../.devtools}/scripts/fuzz.sh" "$@"
