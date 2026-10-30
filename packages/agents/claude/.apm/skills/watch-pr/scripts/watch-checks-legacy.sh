#!/usr/bin/env bash
# watch-checks — emit one line per check as it settles, then exit.
#
# Written as an event stream. Under Claude Code the Monitor tool turns
# each stdout line into a notification; under Codex one long-running
# exec session reads them as they arrive. Either way the caller learns
# about the first failure when it happens rather than after the slowest
# job finishes, and waits without spending a turn per look.
#
# The caller needs:
#
#   * one line per event, flushed as it happens
#   * every terminal state emits, not only the passing one, because a
#     watcher that prints nothing on failure is indistinguishable from a
#     watcher that is still waiting
#   * the command exits once nothing is pending, so the watch ends by
#     itself rather than sitting armed until the timeout
#
# Each poll reads the checks once, through one gh call, and a response
# that isn't a list of checks stops the watch with an error rather than
# reading as "nothing pending". A rerun replaces the run it repeats: the
# newest run per workflow and job name is the one reported, keyed on
# both so two workflows sharing a job name stay apart.
#
# It also works as a plain blocking call, where the exit code carries
# the outcome: 0 all passed, 1 something failed, 2 inconclusive (the
# wait ran out, no check ever registered, or gh failed).
#
# Usage: watch-checks.sh [pull-request-number]
# Written to bash 3.2.
set -uo pipefail

# --- environment hardening -------------------------------------------
# The agent reads this output, so the operator's preferences must not
# change its shape. LC_ALL pins collation, because sort and the [a-z]
# ranges below mean different things under a UTF-8 locale. The unsets
# cover variables that silently retarget a command: GH_REPO sends gh at
# another repository, CDPATH makes a relative cd print somewhere else.
export LC_ALL=C
export GH_PAGER=cat
export GH_PROMPT_DISABLED=1
export PYTHONUTF8=1
unset CDPATH GH_REPO GH_HOST GREP_OPTIONS
IFS=$' \t\n'

# shellcheck disable=SC1091 # generated beside this script
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

readonly INTERVAL=${PR_CHECKS_INTERVAL:-30}
readonly TIMEOUT=${PR_CHECKS_TIMEOUT:-1800}
readonly APPEAR_TIMEOUT=${PR_CHECKS_APPEAR_TIMEOUT:-120}

root=$(git rev-parse --show-toplevel)
cd "$root" || exit 2

fail() {
  printf 'ERROR %s\n' "$1"
  exit 2
}

command -v gh >/dev/null 2>&1 || fail "gh is not installed"
command -v jq >/dev/null 2>&1 || fail "jq is not installed"
dt_gh_login >/dev/null || exit $?

number=${1:-}
if [ -z "$number" ]; then
  branch=$(git rev-parse --abbrev-ref HEAD)
  number=$(dt_gh pr list --head "$branch" --state open --json number --jq '.[0].number') || exit $?
  [ -n "$number" ] || fail "no open pull request for ${branch}"
fi

printf 'WATCHING #%s\n' "$number"

# gh sorts checks into buckets: pass, fail, pending, skipping, cancel.
# Anything outside pending has finished moving.
#
# gh answers a pull request with no checks yet by failing, with "no
# checks reported" on stderr, rather than with an empty list. That one
# failure means an empty set; any other is an error.
gh_err=$(mktemp)
trap 'rm -f "$gh_err"' EXIT
snapshot() {
  if raw_checks=$(dt_gh pr checks "$number" --json name,bucket,link,workflow 2>"$gh_err"); then
    :
  else
    status=$?
    if [ "$status" -eq 76 ]; then
      cat "$gh_err" >&2
      exit 76
    fi
    # shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
    grep -q 'no checks reported' "$gh_err" ||
      fail "could not read checks for #${number}: $(head -n 1 "$gh_err")"
    raw_checks='[]'
  fi
  printf '%s\n' "$raw_checks" | jq -e 'type == "array"' >/dev/null 2>&1 ||
    fail "gh returned an invalid checks response for #${number}"
  checks=$(printf '%s\n' "$raw_checks" | jq '
    map(. + {sort_id: ((((.link // "") | capture("(?<id>[0-9]+)$") | .id) // "0") | tonumber)})
    | sort_by(.workflow, .name, .sort_id)
    | group_by(.workflow, .name)
    | map(last | del(.sort_id))
  ') || fail "could not normalize checks for #${number}"
}

# A pull request opened seconds ago reports no checks at all. Waiting for
# the first one to register keeps an empty set from reading as success.
waited=0
while [ "$waited" -lt "$APPEAR_TIMEOUT" ]; do
  snapshot
  check_count=$(printf '%s\n' "$checks" | jq 'length') || fail "could not count checks for #${number}"
  [ "$check_count" != "0" ] && break
  sleep 5
  waited=$((waited + 5))
done
snapshot
check_count=$(printf '%s\n' "$checks" | jq 'length') || fail "could not count checks for #${number}"
if [ "$check_count" = "0" ]; then
  printf 'NO CHECKS #%s registered after %ss\n' "$number" "$APPEAR_TIMEOUT"
  exit 2
fi

prev=""
seen=""
elapsed=0
failures=0

while :; do
  snapshot
  pending=$(printf '%s\n' "$checks" |
    jq '[.[] | select(.bucket == "pending")] | length')
  current=$(printf '%s\n' "$checks" |
    jq -r --argjson pending "$pending" \
      '.[] | select(.bucket != "pending" and (.bucket != "cancel" or $pending == 0)) | "\(.bucket)\t\(.workflow):\(.name)\t\(.name)\t\(.link)"' |
    sort)

  # Emit only what has settled since the previous look.
  #
  # A seen-set rather than comm. comm compares under the collation its
  # locale defines, so a name carrying spaces or punctuation could sort
  # one way and compare another, and the mismatch drops a FAIL line
  # instead of reporting it. Silence and success would then look alike,
  # which is the one thing this script exists to prevent.
  while IFS="$(printf '\t')" read -r bucket key name link; do
    [ -n "$name" ] || continue
    case "$seen" in
    *"|${key}|"*) continue ;;
    esac
    seen="${seen}|${key}|"
    case $bucket in
    pass) printf 'PASS  %s\n' "$name" ;;
    skipping) printf 'SKIP  %s\n' "$name" ;;
    fail | cancel)
      failures=$((failures + 1))
      printf 'FAIL  %s  %s\n' "$name" "$link"
      ;;
    *) printf '%s  %s\n' "$bucket" "$name" ;;
    esac
  done <<EOF
$current
EOF

  prev=$current

  if [ "$pending" = "0" ]; then
    break
  fi

  if [ "$elapsed" -ge "$TIMEOUT" ]; then
    printf 'TIMEOUT #%s after %ss with checks still running\n' "$number" "$TIMEOUT"
    exit 2
  fi
  sleep "$INTERVAL"
  elapsed=$((elapsed + INTERVAL))
done

# Recount from the final state rather than trusting the running tally,
# which a re-run of a failed job would have left stale.
failures=$(printf '%s\n' "$prev" | grep -c -E '^(fail|cancel)' || true)

if [ "$failures" = "0" ]; then
  printf 'ALL GREEN #%s\n' "$number"
  exit 0
fi

printf 'FAILED #%s with %s failing check(s). Hand it to the fix-pr skill.\n' "$number" "$failures"
exit 1
