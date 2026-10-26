#!/usr/bin/env bash
set -euo pipefail
export DEVTOOLS_UNATTENDED=1
# @include lib/merge-bootstrap.sh
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
bash "$here/../../merge-pr/scripts/lint-squash-msg.sh" "$@"
