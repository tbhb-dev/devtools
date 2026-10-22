#!/usr/bin/env bash
# Scan all refs in CI, or captured branch history and the full index locally.
# cspell:words tuxdb
set -euo pipefail

fail() {
  echo "gitleaks: $*" >&2
  exit 1
}
mode=ci
base=
while [ "$#" -gt 0 ]; do
  case $1 in
  --mode | --base)
    [ "$#" -ge 2 ] || fail "$1 requires a value"
    case $1 in
    --mode) mode=$2 ;;
    --base)
      base=$2
      [ -n "$base" ] || fail '--base requires a ref'
      ;;
    esac
    shift 2
    ;;
  *) fail "unknown argument: $1" ;;
  esac
done
case $mode in
local | ci) ;;
*) fail "unknown mode: $mode" ;;
esac
[ "$mode" = local ] || [ -z "$base" ] || fail '--base requires --mode local'
root=$(git rev-parse --show-toplevel) || fail 'cannot resolve repository root'
cd "$root"
head_oid=$(git rev-parse --verify 'HEAD^{commit}') || fail 'HEAD must name a commit'
printf 'gitleaks mode: %s\nHEAD: %s\n' "$mode" "$head_oid"

if [ "$mode" = local ]; then
  shallow=$(git rev-parse --is-shallow-repository) || fail 'cannot check history depth'
  [ "$shallow" = false ] || fail 'shallow history is incomplete. Fetch the complete origin history and retry.'
  if [ -z "$base" ]; then
    base=$(git symbolic-ref refs/remotes/origin/HEAD) ||
      fail 'repair refs/remotes/origin/HEAD to name the origin default branch and retry.'
  fi
  case $base in
  refs/remotes/origin/HEAD) fail 'base must name an origin branch, not origin/HEAD' ;;
  refs/remotes/origin/?*) ;;
  *) fail 'base must be a fully qualified branch ref under refs/remotes/origin/' ;;
  esac
  git check-ref-format "$base" || fail "invalid base ref: $base"
  base_oid=$(git rev-parse --verify "$base^{commit}") ||
    fail "missing origin base $base. Fetch the required origin history and retry."
  git merge-base "$base_oid" "$head_oid" >/dev/null ||
    fail 'HEAD and base need a common ancestor. Fetch the required origin history and retry.'
  tree_oid=$(git write-tree) || fail 'cannot capture the full index tree'
  snapshot_oid=$(printf 'Gitleaks index snapshot\n' |
    GIT_AUTHOR_NAME=Gitleaks GIT_AUTHOR_EMAIL=gitleaks@example.invalid \
      GIT_COMMITTER_NAME=Gitleaks GIT_COMMITTER_EMAIL=gitleaks@example.invalid \
      GIT_AUTHOR_DATE='2000-01-01T00:00:00Z' GIT_COMMITTER_DATE='2000-01-01T00:00:00Z' \
      git -c commit.gpgSign=false commit-tree "$tree_oid") || fail 'cannot create index snapshot'
  printf 'base ref: %s\nbase commit: %s\nindex tree: %s\n' "$base" "$base_oid" "$tree_oid"
fi

scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT

# Validate the same selection independently, then retain the scanner's status.
# Gitleaks can log Git errors while returning zero and claiming no findings.
scan() {
  local label=$1 opts=$2 status=0
  shift 2
  printf '\nScanning %s\n' "$label"
  git log -p -U0 --format= "$@" >"$scratch/patch" ||
    fail "cannot read $label patch content"
  if [ "$mode" = ci ]; then
    gitleaks git --verbose --redact . >"$scratch/log" 2>&1 || status=$?
  else
    gitleaks git --verbose --redact --log-opts="$opts" . >"$scratch/log" 2>&1 || status=$?
  fi
  cat "$scratch/log"
  [ "$status" -eq 0 ] || return "$status"
  sed $'s/\033\\[[0-9;]*m//g' "$scratch/log" >"$scratch/plain"
  if grep -qE '(^|[[:space:]])(ERR|FTL)([[:space:]]|$)|(^|[[:space:]])(fatal|error):' "$scratch/plain"; then
    fail "scanner reported an error in $label"
  fi
  if ! grep -qE '[1-9][0-9]* commits scanned' "$scratch/log"; then
    if grep -qE '^[+-]' "$scratch/patch"; then
      fail "gitleaks walked 0 commits despite expected patch content in $label"
    fi
    echo "Git selection has no content to scan."
  fi
}

if [ "$mode" = ci ]; then
  scan 'all refs' '' --full-history --all --diff-filter=tuxdb
else
  scan 'branch history' "--full-history --diff-merges=first-parent $base_oid..$head_oid" \
    --full-history --diff-merges=first-parent "$base_oid..$head_oid"
  scan 'staged tree' "--full-history $snapshot_oid" --full-history "$snapshot_oid"
fi
