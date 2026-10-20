#!/usr/bin/env bash
# squash-merge — the one thing in this workflow that merges a pull
# request, and the one that cleans up afterwards.
#
# GitHub will happily write the squash message itself, by concatenating
# every commit on the branch. That output has never passed a commit-msg
# hook, and it lands on the default branch where the whole toolchain
# assumes those hooks ran. So this workflow supplies the message, which
# means the message needs the same gates a commit gets: the linters, and
# a reviewer that did not write it.
#
# In order, it refuses unless: the draft names this pull request; the
# review stamp covers the draft's bytes, the pull request, its live
# head, its base branch, and the commit that base points at; gh acts as
# the configured gh_login, when one is configured; the pull request is
# open, ready, mergeable, CLEAN, approved where a review is required,
# and green by the latest run of every check; the commit-msg gates pass;
# and somebody confirmed the merge (the operator flag, a merge
# pre-approval for this session, or the machine's delegation check, in
# that order). The confirmation is another program, so the stamp and the
# live pull request are checked again after it. The merge itself names
# the reviewed head, so a commit pushed during these checks is refused
# by GitHub rather than squashed in, and it sends this script's own copy
# of the stamped bytes.
#
# Cleanup lands here for the same reason create-pr owns its own: there
# is no post-merge hook to hang it on. The drafts that the pull request
# workflow left behind become garbage the moment the branch merges, and
# this is the code that watched that happen. A branch that lives in a
# fork is never touched: a same-named branch on origin is someone else's.
#
# Usage: squash-merge.sh [--confirmed] [number]
#   --confirmed  the operator answered yes to this merge in this session
# Exit:  0 merged, 1 refused or failed, 75 held: nobody has confirmed the
#        merge; ask, then pass --confirmed; or merge queued/pending
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

readonly SQUASH_DRAFT=SQUASH_AGENTMSG
readonly DESC_DRAFT=PR_AGENTDESC.md

die() {
  printf 'squash-merge: %s\n' "$1" >&2
  exit 1
}

step() { printf '\n--- %s\n' "$1"; }

usage="usage: squash-merge.sh [--confirmed] [number]"
confirmed=false
number=""
expected_head=""
for arg in "$@"; do
  case $arg in
  --confirmed) confirmed=true ;;
  --expected-head=*)
    expected_head=${arg#*=}
    case $expected_head in "" | *[!0-9a-f]*) die "Expected head must be a full commit SHA" ;; esac
    [ "${#expected_head}" -eq 40 ] || die "Expected head must be a full commit SHA"
    ;;
  -* | *[!0-9]*) die "$usage" ;;
  *)
    [ -z "$number" ] || die "$usage"
    number=$arg
    ;;
  esac
done

command -v commit-lint >/dev/null 2>&1 ||
  die 'commit-lint is not installed; add "go:github.com/tbhb-dev/devtools/cmd/commit-lint" = "<devtools-version>" to mise.toml [tools]'
mp_need_tools
root=$(dt_root)
cd "$root"
state=$(dt_state_dir)
ma_context
if [ "${DT_UNATTENDED:-0}" = 1 ] && [ "$confirmed" = true ]; then
  die "Unattended workflows refuse --confirmed; use pre-approval or delegation."
fi
here=$DT_LIB_DIR

branch=$(gitr rev-parse --abbrev-ref HEAD)
if [ -z "$number" ]; then
  number=$(dt_gh pr list --head "$branch" --state open --json number --jq '.[0].number // empty') || exit $?
  [ -n "$number" ] || die "no open pull request for ${branch}. Pass a number explicitly."
fi

# --- gate 1: the message exists and belongs to this pull request -----

step "checking ${SQUASH_DRAFT}"
[ -s "$SQUASH_DRAFT" ] || die "no squash message drafted. Run squash-message.sh first."

subject=$(head -1 "$SQUASH_DRAFT")
drafted_for=$(printf '%s' "$subject" | sed -n 's/.*(#\([0-9][0-9]*\))[[:space:]]*$/\1/p')
[ "$drafted_for" = "$number" ] ||
  die "${SQUASH_DRAFT} was drafted for #${drafted_for:-unknown}, not #${number}."
printf 'drafted for #%s\n' "$number"

# --- gate 2: the review stamp, against the live pull request ---------

view=$state/merge.view.json
read_live() {
  mp_view "$number" "$view" \
    "$MA_GATE_FIELDS,reviewDecision,headRefName,isCrossRepository" || exit $?
  head_oid=$(mp_field "$view" headRefOid)
  base_ref=$(mp_field "$view" baseRefName)
  base_oid=$(mp_base_oid "$base_ref") || exit $?
}

require_reviewed() {
  [ -f "$state/squash.stamp" ] ||
    die "no independent review has cleared this message. Run it, resolve what
it returns, then retry:

  bash $here/review.sh $number"
  (dt_require_stamp squash "$SQUASH_DRAFT" pr "$number" head "$head_oid" base "$base_ref" base_oid "$base_oid") ||
    die "the reviewed message and pull request are no longer the ones about to
merge. Review again against the current draft and branch:

  bash $here/review.sh $number"
}

step "checking the review stamp"
read_live
require_reviewed
reviewed_head=$(dt_stamp_value squash head)
[ -z "$expected_head" ] || [ "$expected_head" = "$reviewed_head" ] || die "Expected head differs from the reviewed head"
printf 'reviewed at head %s, onto %s at %s\n' "$reviewed_head" "$base_ref" "$base_oid"

# --- gate 3: the account gh acts as ----------------------------------
#
# A merge cannot be undone, so it goes out under the configured account
# or not at all. Without a configured gh_login there is nothing to
# compare against, and the output says the check was skipped.

step "checking the gh account"
want_login=$(dt_conf gh_login)
if [ -z "$want_login" ]; then
  printf 'gh_login is not configured; skipping the account check\n'
else
  login=$(dt_gh_login) || exit $?
  [ "$login" = "$want_login" ] ||
    die "gh is signed in as ${login:-nobody}, not the configured ${want_login}. Nothing was merged."
  printf 'signed in as %s\n' "$login"
fi

# --- gate 4: the pull request is actually mergeable -------------------

step "checking #${number}"

# GitHub recomputes mergeability whenever the base moves, and reports
# UNKNOWN for the few seconds that takes, so the second of two merges in
# a row reads UNKNOWN almost every time. Wait that out through the same
# snapshot, then judge whatever settled. CONFLICTING and every other
# blocking state still refuse below, and so does a wait that runs out.
wait_limit=${MERGE_MERGEABLE_WAIT:-60}
poll_every=${MERGE_MERGEABLE_POLL:-3}
waited=0
while :; do
  mergeable=$(mp_field "$view" mergeable) || exit $?
  merge_state=$(mp_field "$view" mergeStateStatus) || exit $?
  [ "$mergeable" = UNKNOWN ] || [ "$merge_state" = UNKNOWN ] || break
  [ "$waited" -lt "$wait_limit" ] ||
    die "#${number} still reports its mergeability as UNKNOWN after ${wait_limit}s. Try again shortly."
  [ "$waited" -gt 0 ] || printf 'GitHub is still computing mergeability; waiting up to %ss\n' "$wait_limit"
  sleep "$poll_every"
  waited=$((waited + (poll_every > 0 ? poll_every : 1)))
  read_live
done
[ "$waited" = 0 ] || require_reviewed

pr_state=$(mp_field "$view" state)
[ "$pr_state" = OPEN ] || die "#${number} is ${pr_state}, not OPEN."
is_draft=$(mp_field "$view" isDraft) || exit $?
[ "$is_draft" = false ] || die "#${number} is a draft. Mark it ready before merging."
mergeable=$(mp_field "$view" mergeable)
[ "$mergeable" = MERGEABLE ] || die "#${number} is not mergeable (${mergeable:-unknown})."
mp_require_reviews "$view" "$number"
merge_state=$(mp_field "$view" mergeStateStatus)
[ "$merge_state" = CLEAN ] ||
  die "#${number} is not ready to merge (${merge_state:-unknown}). BEHIND or DIRTY goes to the rebase skill."
cross_repo=$(mp_field "$view" isCrossRepository)
case $cross_repo in
true | false) ;;
*) die "cannot determine whether #${number} comes from a fork; branch cleanup is unsafe." ;;
esac

status=0
judged=$(mp_judge_checks "$view") || status=$?
case $status in
0) ;;
1)
  printf '%s\n' "$judged" | awk -F'\t' '$1 == "fail" { printf "%-14s %s\n", $2, $3 }' >&2
  die "#${number} has a failing check, judged by its latest run. Hand it to the fix-pr skill."
  ;;
2)
  printf '%s\n' "$judged" | awk -F'\t' '$1 == "pending" { printf "%-14s %s\n", $2, $3 }' >&2
  die "#${number} has a check still running. Wait through the watch-pr skill."
  ;;
*) die "the checks of #${number} could not be read." ;;
esac
printf 'open, ready, mergeable, and green\n'

# --- gate 5: the commit-msg linters ----------------------------------
#
# The same four hooks a commit answers to. They run here rather than
# only in the skill body because this message never passes through
# git's commit-msg hook: GitHub writes the commit, not this machine.
# There is no fallback: without the task, nothing merges.

step "running the commit-msg gates"
if [ "${DT_UNATTENDED:-0}" != 1 ]; then
  command -v mise >/dev/null 2>&1 || die "mise is not installed; the squash-message gate cannot run."
  mise task info lint-squash-msg >/dev/null 2>&1 ||
    die "mise run lint-squash-msg is absent; the squash-message gate cannot run."
fi
ma_lint "$reviewed_head" ||
  die "the squash message fails the commit-msg gates. Fix it, review it again, retry."
commit-lint --rules trailers --require-assisted-from "$base_oid..$reviewed_head" "$SQUASH_DRAFT" ||
  die "the squash message lacks attribution required by the commits it collapses. Fix it, review it again, retry."

# --- gate 6: somebody confirmed the merge ----------------------------
#
# A bare `mise run preapprove` grants commit and pr and never merge; a
# merge pre-approval has to name merge, because it is the one action
# here that no later step can walk back.

ma_gates "$view" "$number" "$base_oid"

step "confirming"
status=0
set --
if [ "${DT_UNATTENDED:-0}" = 1 ] && jq -e '.owner_paths | length > 0' "$state/merge.policy.json" >/dev/null; then
  set -- --owner-path
fi
dt_confirm merge "$confirmed" "$@" || status=$?
case $status in
0) ;;
1) exit 1 ;;
*) exit "$DT_HELD" ;;
esac

# The confirmation may have run another program, and the linters are
# others too. Read the pull request and the stamp again, then merge
# from a copy of the bytes the stamp covers.
read_live
require_reviewed
ma_gates "$view" "$number" "$base_oid"

# --- merge -----------------------------------------------------------

head_ref=$(mp_field "$view" headRefName)
ma_prepare_children
# Retargeting children is another write: pin and judge the live state again.
if [ "${DT_UNATTENDED:-0}" = 1 ]; then
  read_live
  require_reviewed
  ma_gates "$view" "$number" "$base_oid"
  # Readiness is repository code. Re-read live approvals and checks after it.
  read_live
  require_reviewed
  ma_gates "$view" "$number" "$base_oid" true
  require_reviewed
fi
message=$state/merge.message
cp "$SQUASH_DRAFT" "$message"
message_digest=$(dt_digest "$message") || exit $?
reviewed_digest=$(dt_stamp_value squash draft) || exit $?
[ "$message_digest" = "$reviewed_digest" ] ||
  die "${SQUASH_DRAFT} changed while it was being copied. Review it again."
subject=$(head -1 "$message")
body=$state/merge.body
awk 'NR > 1' "$message" | awk 'NF { seen = 1 } seen' >"$body"
[ -s "$body" ] || die "the message has a subject and no body; a squash message says why."

step "squashing #${number} into ${base_ref}"
merge_status=0
dt_gh pr merge "$number" --squash --match-head-commit "$reviewed_head" \
  --subject "$subject" --body-file "$body" || merge_status=$?

# A successful command may only enqueue the merge, and an error may
# follow a completed server-side merge. Read once; never repeat the write.
result=$state/merge.result.json
check_again="Check with: gh pr view $number --json state,headRefOid,mergeCommit"
if ! mp_view "$number" "$result" state,headRefOid,mergeCommit,url; then
  die "UNKNOWN: could not confirm #${number} after gh exited ${merge_status}. Drafts and stamps kept. $check_again"
fi
result_state=$(mp_field "$result" state) || die "UNKNOWN: invalid merge result. $check_again"
result_head=$(mp_field "$result" headRefOid) || die "UNKNOWN: invalid merge result. $check_again"
merge_commit=$(jq -r '.mergeCommit.oid // empty' "$result") || die "UNKNOWN: invalid merge result. $check_again"
case $result_state in
MERGED)
  [ "$result_head" = "$reviewed_head" ] && [ -n "$merge_commit" ] ||
    die "UNKNOWN: #${number} reports MERGED without the reviewed head and a merge commit. Drafts and stamps kept. $check_again"
  printf 'MERGED: #%s at %s (gh exit %s); recorded in %s\n' "$number" "$merge_commit" "$merge_status" "$result"
  ;;
OPEN)
  if [ "$merge_status" -eq 0 ]; then
    printf 'PENDING: #%s is queued or pending. Drafts and stamps kept. %s\n' "$number" "$check_again"
    exit "$DT_HELD"
  fi
  die "FAILED: gh exited ${merge_status} and #${number} remains OPEN. Drafts and stamps kept. $check_again"
  ;;
CLOSED)
  die "FAILED: #${number} is CLOSED without merging. Drafts and stamps kept."
  ;;
*)
  die "UNKNOWN: unexpected state ${result_state:-missing} for #${number}. Drafts and stamps kept. $check_again"
  ;;
esac

ma_verify
ma_report

# --- clean up --------------------------------------------------------

step "cleaning up"

head_ref=$(mp_field "$view" headRefName)
rm -f "$message" "$body" "$view"

# The drafts describe a pull request that no longer exists. Removed
# rather than emptied, so a stale read cannot carry them forward. The
# two .reviewed files are the stamps this toolchain wrote before the
# review moved under agent-skills/.
git_dir=$(dt_git_dir)
rm -f "$SQUASH_DRAFT" "$DESC_DRAFT" "$git_dir/squash-agentmsg.reviewed" "$git_dir/pr-agentdesc.reviewed"
dt_clear squash
dt_clear pr
printf 'removed %s and %s\n' "$SQUASH_DRAFT" "$DESC_DRAFT"

# The remote branch goes; the local one may be the branch this worktree
# stands on, and deleting that would strand the session. Say so instead.
if [ "$cross_repo" = false ] && git ls-remote --exit-code --heads origin "$head_ref" >/dev/null 2>&1; then
  if deleted=$(git push --force-with-lease="refs/heads/$head_ref:$reviewed_head" origin ":refs/heads/$head_ref" 2>&1); then
    printf 'deleted origin/%s\n' "$head_ref"
  elif printf '%s\n' "$deleted" | grep -Fq 'remote ref does not exist'; then
    printf 'origin/%s was already deleted\n' "$head_ref"
  else
    printf 'could not delete origin/%s; a ruleset may protect it\n' "$head_ref"
  fi
fi

git fetch --quiet origin "$base_ref" 2>/dev/null || true
if [ "$cross_repo" = true ]; then
  printf 'left fork branch %s untouched\n' "$head_ref"
elif [ "$branch" = "$head_ref" ]; then
  printf '\nThis worktree still stands on %s, which has merged.\n' "$head_ref"
  printf 'Switch to %s before deleting it, or drop the whole worktree.\n' "$base_ref"
elif [ "${DT_UNATTENDED:-0}" != 1 ] && gitr rev-parse --verify --quiet "refs/heads/$head_ref" >/dev/null; then
  git branch -D "$head_ref" >/dev/null 2>&1 &&
    printf 'deleted local %s\n' "$head_ref" || true
fi

if [ "${DT_UNATTENDED:-0}" = 1 ] && [ "$cross_repo" = false ]; then
  disposable=$(dt_repo_conf disposable_ignored)
  ma_policy cleanup "$base_ref" "$head_ref" "$reviewed_head" "$merge_commit" "$disposable" || printf 'Merged; local cleanup incomplete, inspect retained state.\n' >&2
fi
step "done"
jq -r '"merged: \(.url) (\(.state)), commit \(.mergeCommit.oid)"' "$result"
