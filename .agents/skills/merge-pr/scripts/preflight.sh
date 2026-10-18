#!/usr/bin/env bash
# preflight — gather every fact the merge-pr skill needs before it
# writes a squash message or merges anything.
#
# It answers the questions that decide whether a merge should happen at
# all (state, draft status, review decision, checks, mergeability), the
# ones that shape the message (title, description, the commits being
# collapsed), and who can confirm the merge besides the operator.
#
# This skill also runs against pull requests nobody here authored, a
# dependency bump being the usual case, so the report never assumes the
# branch is checked out locally. Everything about the pull request comes
# from one gh response, and the checks are judged by the same reader the
# merge script uses, so the two never disagree about green.
#
# Nothing here mutates anything. Usage: preflight.sh [number]
set -euo pipefail

# --- environment hardening -------------------------------------------
# lib.sh repeats this block; it stays here too so the hygiene gate can
# see it in the file that calls git and gh.
export LC_ALL=C
export GH_PAGER=cat
export GH_PROMPT_DISABLED=1
export PYTHONUTF8=1
unset CDPATH GH_REPO GH_HOST GREP_OPTIONS
IFS=$' \t\n'

# shellcheck disable=SC1091 # lib.sh is generated beside this script
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
# shellcheck disable=SC1091 # generated beside this script
. "$DT_LIB_DIR/pull-request.sh"

readonly DRAFT=SQUASH_AGENTMSG
readonly COMMIT_CAP=${MERGE_PREFLIGHT_COMMITS:-30}

section() { printf '\n== %s ==\n' "$1"; }
none() { printf '(none)\n'; }

root=$(dt_root)
cd "$root"
ma_context

section "gh"
gh_state=ok
if ! command -v gh >/dev/null 2>&1; then
  gh_state="gh is not installed"
elif ! command -v jq >/dev/null 2>&1; then
  gh_state="jq is not installed"
fi
printf '%s\n' "$gh_state"
if [ "$gh_state" != ok ]; then
  printf 'Stop here and tell the operator.\n'
  exit 0
fi

login=$(dt_gh_login) || exit $?

branch=$(gitr rev-parse --abbrev-ref HEAD)
number=${1:-}
if [ -z "$number" ]; then
  number=$(dt_gh pr list --head "$branch" --state open --json number --jq '.[0].number // empty') || exit $?
fi

section "target"
printf 'local branch: %s\n' "$branch"
if [ -z "$number" ]; then
  printf 'pull request: NONE for this branch, and none given.\n'
  printf 'Pass the number as the skill argument, or open one with the pr skill.\n'
  printf '\nopen pull requests here:\n'
  dt_gh pr list --state open --limit 20 --json number,title,author,isDraft \
    --jq '.[] | "#\(.number) \(.title) (\(.author.login))" + (if .isDraft then " [draft]" else "" end)' || exit $?
  exit 0
fi
printf 'pull request: #%s\n' "$number"

view=$(dt_state_dir)/merge-preflight.json
mp_view "$number" "$view" \
  number,title,state,isDraft,author,baseRefName,headRefName,headRefOid,isCrossRepository,mergeable,mergeStateStatus,reviewDecision,url,statusCheckRollup,commits,body,files,labels,reviews || exit $?

section "state"
jq -r '
  "title:  \(.title)",
  "state:  \(.state)" + (if .isDraft then " (DRAFT — mark it ready before merging)" else "" end),
  "author: \(.author.login)",
  "\(.headRefName) into \(.baseRefName)" + (if .isCrossRepository then " (from a fork)" else "" end),
  "head oid: \(.headRefOid)",
  "mergeable: \(.mergeable) / \(.mergeStateStatus)",
  "review decision: " + (if (.reviewDecision // "") == "" then "(none required)" else .reviewDecision end),
  "url: \(.url)"
' "$view"

mp_require_reviews "$view" "$number"
if [ "${DT_UNATTENDED:-0}" = 1 ]; then
  pinned_base=$(mp_field "$view" baseRefName)
  pinned_base=$(mp_base_oid "$pinned_base")
  ma_gates "$view" "$number" "$pinned_base"
fi

section "checks"
status=0
judged=$(mp_judge_checks "$view") || status=$?
if [ "$status" = 3 ]; then
  printf 'the check rollup could not be read\n'
elif [ -z "$judged" ]; then
  printf 'no checks reported\n'
else
  printf '%s\n' "$judged" | sort -t"$(printf '\t')" -k3 | awk -F'\t' '{ printf "%-14s %s\n", $2, $3 }'
  superseded=$(mp_superseded "$view")
  if [ "$superseded" != 0 ]; then
    printf '(%s earlier run(s) left out: a later run of the same check replaced each)\n' "$superseded"
  fi
  printf '\nsummary: '
  case $status in
  0) printf 'all green.\n' ;;
  1) printf 'FAILING. Do not merge. Hand this to the fix-pr skill instead.\n' ;;
  2) printf 'still running. Wait through the watch-pr skill before merging.\n' ;;
  esac
fi

section "commits being collapsed"
# shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
printf 'count: %s\n\n' "$(jq -r '.commits | length' "$view")"
jq -r '.commits[] | "--- " + .oid[0:7] + "\n" + .messageHeadline + "\n" + (.messageBody // "")' "$view" |
  head -n "$((COMMIT_CAP * 8))" || true

section "description as published"
jq -r '.body // ""' "$view"

section "files"
jq -r '.files[]? | "\(.additions)+ \(.deletions)- \(.path)"' "$view"
rm -f "$view"

# The account gh acts as. A merge cannot be undone, so squash-merge.sh
# refuses one made from any account but the configured one.
section "identity"
want_login=$(dt_conf gh_login)
if [ -z "$want_login" ]; then
  printf 'gh account: %s; gh_login is not configured, so the merge does not check it\n' "${login:-unknown}"
elif [ "$login" = "$want_login" ]; then
  printf 'gh account: %s, the configured gh_login\n' "$login"
else
  printf 'gh account: %s, NOT the configured gh_login %s — the merge refuses until they match\n' \
    "${login:-unknown}" "$want_login"
fi

# Who can answer step 6 besides the operator. squash-merge.sh resolves
# this itself at merge time, in the same order. A grant comes from
# `mise run preapprove merge`, keyed on the session; a bare preapprove
# never implies it, because a merge is the one action in this toolchain
# that no later step can walk back. A delegation comes from the
# machine's delegation_check, reported here as configured rather than
# called, because asking it is the merge's job.
section "pre-approval"
session=$(dt_session_id) || exit $?
if [ -n "$session" ] && grant=$(dt_preapproval merge); then
  case ${grant##* } in
  touchid) how="a Touch ID prompt stands behind it" ;;
  *) how="no biometric prompt stood behind it" ;;
  esac
  printf 'merge: GRANTED, %s — step 6 skips the confirmation once every gate and the review pass\n' "$how"
else
  printf 'merge: not granted — merge holds until an authority grants it\n'
fi
checker=$(dt_conf delegation_check)
if [ -n "$checker" ]; then
  printf 'delegation: %s is configured; squash-merge.sh asks it when nobody else has answered\n' "$checker"
else
  printf 'delegation: none configured\n'
fi

section "review"
# shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
printf 'harness:  %s\n' "$(dt_harness)"
# shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
printf 'reviewer: %s\n' "$(dt_reviewer_label)"
state=$(dt_state_dir) || exit $?
if [ -f "$state/squash.stamp" ]; then
  # shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
  printf 'stamp:    present for #%s from an earlier review; review.sh replaces it\n' \
    "$(dt_stamp_value squash pr)"
else
  printf 'stamp:    none\n'
fi

section "preconditions"
if git check-ignore --quiet "$DRAFT" 2>/dev/null; then
  printf '%s gitignored: yes\n' "$DRAFT"
else
  printf '%s gitignored: NO — add it to .gitignore before drafting\n' "$DRAFT"
fi
if [ "${DT_UNATTENDED:-0}" = 1 ]; then
  configured_lint=$(dt_repo_conf lint_squash_cmd 'mise run lint-squash-msg')
  [ -n "$configured_lint" ] || dt_die "lint_squash_cmd must not be empty"
  printf 'configured squash lint: %s\n' "$configured_lint"
elif command -v mise >/dev/null 2>&1 && mise task info lint-squash-msg >/dev/null 2>&1; then
  printf 'mise run lint-squash-msg: present\n'
else
  printf 'mise run lint-squash-msg: ABSENT — stop and tell the operator\n'
fi
if [ -f "$DT_LIB_DIR/../../review-squash-message/SKILL.md" ]; then
  printf 'review-squash-message skill deployed: yes\n'
else
  printf 'review-squash-message skill deployed: NO — run apm install\n'
fi

# A leftover squash draft belongs to whichever pull request last used
# this workflow, and merging one message into another pull request is
# the worst outcome available here. Say which it was drafted for, and
# leave it: a briefing deletes nothing, and squash-message.sh replaces
# it when drafting begins.
if [ ! -s "$DRAFT" ]; then
  printf '%s: absent (expected before drafting)\n' "$DRAFT"
else
  drafted_for=$(head -1 "$DRAFT" | sed -n 's/.*(#\([0-9][0-9]*\))[[:space:]]*$/\1/p')
  if [ "$drafted_for" = "$number" ]; then
    printf '%s: present, drafted for #%s\n' "$DRAFT" "$number"
  else
    printf '%s: STALE. It was drafted for #%s, not #%s.\n' \
      "$DRAFT" "${drafted_for:-an unknown pull request}" "$number"
    printf 'Leave it untouched here; squash-message.sh replaces it when drafting begins.\n'
  fi
fi
