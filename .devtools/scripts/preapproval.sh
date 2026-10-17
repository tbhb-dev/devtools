#!/usr/bin/env bash
# Shared scope validation and read-only lookup for consumer tasks.
set -euo pipefail

preapproval_scopes() {
  local scope extra declared configured=${DEVTOOLS_PREAPPROVAL_SCOPES:-}
  local -a extras
  read -r -a extras <<<"${configured//$'\n'/ }"
  for scope in ${extras[@]+"${extras[@]}"}; do
    if [[ ! $scope =~ ^[a-z][a-z0-9-]*$ ]] || [[ $scope == all ]]; then
      echo "invalid configured preapproval scope '$scope'" >&2
      return 1
    fi
  done
  for scope in "$@"; do
    if [[ ! $scope =~ ^[a-z][a-z0-9-]*$ ]]; then
      echo "invalid preapproval scope '$scope'" >&2
      return 1
    fi
    case $scope in
    all) printf '%s\n' commit pr merge ;;
    commit | pr | merge) printf '%s\n' "$scope" ;;
    *)
      declared=0
      for extra in ${extras[@]+"${extras[@]}"}; do
        [[ $extra != "$scope" ]] || declared=1
      done
      if [[ $declared == 0 ]]; then
        echo "unknown scope '$scope'" >&2
        return 1
      fi
      printf '%s\n' "$scope"
      ;;
    esac
  done
}

if [[ ${BASH_SOURCE[0]} == "$0" ]]; then
  if [[ $# != 1 || ${1:-} == all ]]; then
    echo 'usage: preapproval.sh <scope> (one action, not all)' >&2
    exit 1
  fi
  scope=$(preapproval_scopes "$1")
  session=${CLAUDE_CODE_SESSION_ID:-${CODEX_THREAD_ID:-}}
  [[ -n $session ]] || exit 1
  file="$(git rev-parse --absolute-git-dir)/preapprovals/$session"
  [[ -f $file ]] && grep -q "^$scope " "$file"
fi
