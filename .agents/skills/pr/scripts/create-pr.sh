#!/usr/bin/env bash
# create-pr — the one thing in this workflow that publishes a pull
# request description.
#
# The commit workflow could lean on git, whose own entry point carries
# hooks on both sides: one gates what goes in, the other cleans up
# afterwards. Publishing a pull request has no such entry point. Rather
# than guess at an it-worked signal from the outside, this workflow owns
# the inside, so every gate runs here and so does the call they gate.
#
# Running it a second time updates the open pull request rather than
# opening another, and brings its title, body, base, labels, reviewers,
# assignees, milestone, and draft state to what the draft says. A
# description goes stale the moment remediation adds work, and the fix
# for that is the same path that published it, so the same gates apply
# to the rewrite.
#
# In order, it refuses unless: the draft exists, no rebase, merge, or
# cherry-pick is under way, the branch is neither detached nor the base,
# the validator passes, the review stamp covers the draft's bytes, HEAD,
# the resolved base ref, that ref's commit, and the merge base, gh can
# resolve every label, the milestone, and every #N the description
# cites, the push would be a fast-forward (or --rewrite allows the one
# rewrite it can make safely), and somebody confirmed the publish (the
# operator flag, a pre-approval for this session, or the machine's
# delegation check). The confirmation is another program, so the stamp
# is checked again after it, and then gh has to be signed in as the
# configured agent account. Everything published comes from this
# script's own copy of the stamped bytes.
#
# The push is a fast-forward of what origin holds, read from origin
# itself with git ls-remote. A branch rebased after its last push goes
# up only with --rewrite, which leases on that exact remote commit, and
# refuses when HEAD is behind origin or when origin holds a commit this
# checkout never fetched. After the push, HEAD has to still be the
# reviewed commit and origin has to hold it.
#
# The draft outlives this script. PR_AGENTDESC.md stays the working copy
# of the description for as long as the pull request is open, and the
# merge-pr skill removes it once the branch lands.
#
# Usage: create-pr.sh [--confirmed] [--rewrite] [--dry-run] [--check-description] [--rerun-gates]
#   --confirmed  the operator answered yes to this publish in this session
#   --rewrite    the branch was rebased on purpose: replace origin's copy
#   --dry-run    run every check, say what the push would do, and stop
#                before confirming, pushing, or publishing anything
# Exit:  0 published (or the dry run passed), 1 refused, 75 held:
#        nobody has confirmed the publish; ask, then pass --confirmed
# shellcheck disable=SC2034,SC2154 # publication and properties are sourced below
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
# shellcheck disable=SC1091 # description.sh is deployed beside this script
. "$DT_LIB_DIR/description.sh"
# shellcheck disable=SC1091 # unattended behavior shares these entry points
. "$DT_LIB_DIR/pr-agent.sh"

die() {
  printf 'create-pr: %s\n' "$1" >&2
  exit 1
}

step() { printf '\n--- %s\n' "$1"; }

usage="usage: create-pr.sh [--confirmed] [--rewrite] [--dry-run] [--check-description] [--rerun-gates]"
confirmed=false
rewrite=false
dry_run=false
check_description=false
rerun_gates=false
for arg in "$@"; do
  case $arg in
  --confirmed) confirmed=true ;;
  --rewrite) rewrite=true ;;
  --dry-run) dry_run=true ;;
  --check-description) check_description=true ;;
  --rerun-gates) rerun_gates=true ;;
  *) die "$usage" ;;
  esac
done

if [ "${DT_UNATTENDED:-0}" = 1 ] && [ "$confirmed" = true ]; then
  die "Unattended workflows refuse --confirmed; use pre-approval or delegation."
fi
root=$(dt_root)
cd "$root"
git_dir=$(dt_git_dir)
state=$(dt_state_dir)
here=$DT_LIB_DIR
draft=$root/PR_AGENTDESC.md
pr_agent_context

[ -s "$draft" ] || die "no draft at PR_AGENTDESC.md. Draft the description first."
if [ -d "$git_dir/rebase-merge" ] || [ -d "$git_dir/rebase-apply" ] ||
  [ -f "$git_dir/MERGE_HEAD" ] || [ -f "$git_dir/CHERRY_PICK_HEAD" ]; then
  die "a rebase, merge, or cherry-pick is in progress. Finish or abort it first:
a pull request published now would carry a branch the review never saw."
fi
branch=$(gitr symbolic-ref -q --short HEAD) || die "HEAD is detached. Check out the branch to publish."
base=$(pr_field "$draft" base)
[ -n "$base" ] || die "the draft's frontmatter carries no base."
[ "$branch" != "$base" ] || die "${branch} is the base branch. Publish a topic branch."
base_ref=$(pr_base_ref "$base")
pr_agent_range "$base_ref"
[ -n "$base_ref" ] || die "the draft's base, ${base}, names no local or origin branch."

check_identity() {
  local want login
  want=$(dt_conf gh_login)
  [ "${DT_UNATTENDED:-0}" != 1 ] || [ -n "$want" ] || die "gh_login must be configured on the machine"
  if [ -z "$want" ]; then
    printf 'gh identity: gh_login is not configured, so the account check is skipped\n'
    return 0
  fi
  login=$(dt_gh_login) || exit $?
  [ "$login" = "$want" ] ||
    die "gh is signed in as ${login:-nobody}, not ${want}. Nothing was pushed or published."
  printf 'gh identity: %s\n' "$login"
}

check_identity

# Cheap description checks precede scans, review, and tests.
step "validating PR_AGENTDESC.md"
pr_checks "$draft" "$rerun_gates"

# --- gate 2: the independent review ----------------------------------
#
# The stamp has to match everything the review read: the draft's bytes,
# the branch, the base as it was resolved then, and where the two meet.
# A fetch that moved origin's copy of the base voids it as surely as a
# new commit on the branch does.

require_reviewed() {
  [ -f "$state/pr.stamp" ] || die "no independent review has cleared this draft. Run it:

  bash $here/review.sh"
  local now_ref now_oid now_base now_head now_mergebase now_branch
  now_base=$(pr_field "$draft" base) || return $?
  now_ref=$(pr_base_ref "$now_base") || return $?
  now_head=$(dt_head) || return $?
  now_branch=$(gitr symbolic-ref --short HEAD) || return $?
  now_oid=$(gitr rev-parse --verify --quiet "${now_ref:-none}^{commit}" || printf 'none\n')
  now_mergebase=$(gitr merge-base "$now_oid" HEAD 2>/dev/null || printf 'none\n') || return $?
  (dt_require_stamp pr "$draft" head "$now_head" base "$now_ref" base_oid "$now_oid" \
    mergebase "$now_mergebase" branch "$now_branch") ||
    die "the reviewed description and branch are no longer what would publish.
Review it again against the current draft, branch, and base:

  bash $here/review.sh"
}

# shellcheck disable=SC1091 # shares the draft and publication properties
. "$here/properties.sh"
if [ "$check_description" = true ]; then
  rm -f "$copy" "$body_file"
  exit 0
fi
require_reviewed

mise run devtools:lint-leaks
mise run devtools:gitleaks
dt_gate lint "" "$rerun_gates" mise run lint
DT_UNATTENDED=${DT_UNATTENDED:-0} bash "$here/review.sh"
step "checking the review stamp"
require_reviewed
head=$(dt_head)
copy_digest=$(dt_digest "$copy") || exit $?
reviewed_digest=$(dt_stamp_value pr draft) || exit $?
[ "$copy_digest" = "$reviewed_digest" ] ||
  die "PR_AGENTDESC.md changed while it was being copied. Review it again."
printf 'stamped for %s on %s\n' "$head" "$base_ref"

dt_gate test "" "$rerun_gates" mise run test

# shellcheck disable=SC1091 # publication shares the checked state above
. "$here/publish.sh"
