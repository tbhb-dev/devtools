#!/usr/bin/env bash
# Repository-local compatibility entry point; silence means a clean tree.
set -euo pipefail
exec mise --quiet run run agentrepo check-versions "$@"
