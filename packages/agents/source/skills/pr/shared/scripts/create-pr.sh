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
# Usage: create-pr.sh [--confirmed] [--rewrite] [--dry-run]
#   --confirmed  the operator answered yes to this publish in this session
#   --rewrite    the branch was rebased on purpose: replace origin's copy
#   --dry-run    run every check, say what the push would do, and stop
#                before confirming, pushing, or publishing anything
# Exit:  0 published (or the dry run passed), 1 refused, 75 held:
#        nobody has confirmed the publish; ask, then pass --confirmed
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

die() {
  printf 'create-pr: %s\n' "$1" >&2
  exit 1
}

step() { printf '\n--- %s\n' "$1"; }

usage="usage: create-pr.sh [--confirmed] [--rewrite] [--dry-run]"
confirmed=false
rewrite=false
dry_run=false
for arg in "$@"; do
  case $arg in
  --confirmed) confirmed=true ;;
  --rewrite) rewrite=true ;;
  --dry-run) dry_run=true ;;
  *) die "$usage" ;;
  esac
done

root=$(dt_root)
cd "$root"
git_dir=$(dt_git_dir)
state=$(dt_state_dir)
here=$DT_LIB_DIR
draft=$root/PR_AGENTDESC.md

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
[ -n "$base_ref" ] || die "the draft's base, ${base}, names no local or origin branch."

# --- gate 1: the mechanical checks -----------------------------------

step "validating PR_AGENTDESC.md"
bash "$here/validate-description.sh" "PR_AGENTDESC.md" ||
  die "the description does not satisfy the template. Fix the findings above and retry."
printf 'clean\n'

# --- gate 2: the independent review ----------------------------------
#
# The stamp has to match everything the review read: the draft's bytes,
# the branch, the base as it was resolved then, and where the two meet.
# A fetch that moved origin's copy of the base voids it as surely as a
# new commit on the branch does.

require_reviewed() {
  [ -f "$state/pr.stamp" ] || die "no independent review has cleared this draft. Run it:

  bash $here/review.sh"
  local now_ref now_oid
  now_ref=$(pr_base_ref "$(pr_field "$draft" base)")
  now_oid=$(gitr rev-parse --verify --quiet "${now_ref:-none}^{commit}" || printf 'none\n')
  (dt_require_stamp pr "$draft" head "$(dt_head)" base "$now_ref" base_oid "$now_oid" \
    mergebase "$(gitr merge-base "$now_oid" HEAD 2>/dev/null || printf 'none\n')") ||
    die "the reviewed description and branch are no longer what would publish.
Review it again against the current draft, branch, and base:

  bash $here/review.sh"
}

step "checking the review stamp"
require_reviewed
head=$(dt_head)
copy=$state/pr.draft
cp "$draft" "$copy"
[ "$(dt_digest "$copy")" = "$(dt_stamp_value pr draft)" ] ||
  die "PR_AGENTDESC.md changed while it was being copied. Review it again."
printf 'stamped for %s on %s\n' "$head" "$base_ref"

# --- parse the reviewed copy -----------------------------------------

is_draft=$(pr_field "$copy" draft)
labels=$(pr_sequence "$(pr_field "$copy" labels)")
reviewers=$(pr_sequence "$(pr_field "$copy" reviewers)")
assignees=$(pr_sequence "$(pr_field "$copy" assignees)")
milestone=$(pr_field "$copy" milestone)
title=$(pr_title "$copy")
[ -n "$title" ] || die "the draft carries no level 1 heading to use as the title"
body_file=$state/pr.body.md
pr_body "$copy" >"$body_file"
[ -s "$body_file" ] || die "the description has a title but no body"

# --- gate 3: resolve everything the API has to recognize -------------
#
# gh adds labels, reviewers, and the milestone after the pull request
# exists. An unknown one therefore fails *after* creation, leaving a
# published pull request missing the properties it was supposed to
# carry. Resolving them first turns that into a clean refusal.

step "resolving pull request properties"
command -v gh >/dev/null 2>&1 || die "gh is not installed"
gh auth status >/dev/null 2>&1 || die "gh is not authenticated. Run gh auth login."

known_labels=$(gh label list --limit 200 --json name --jq '.[].name') ||
  die "gh could not list the repository's labels"
while IFS= read -r want; do
  [ -n "$want" ] || continue
  printf '%s\n' "$known_labels" | grep -qxF -- "$want" ||
    die "no label named \"${want}\" in this repository. Preflight printed the label set."
done <<EOF
$labels
EOF

if [ -n "$milestone" ]; then
  known_milestones=$(gh api 'repos/{owner}/{repo}/milestones' --jq '.[].title') ||
    die "gh could not list the repository's milestones"
  printf '%s\n' "$known_milestones" | grep -qxF -- "$milestone" ||
    die "no milestone named \"${milestone}\" in this repository."
fi

# Every issue or pull request the description points at has to exist. A
# reference to a number nobody opened is the cheapest kind of invented
# claim, and it is the one a machine can settle.
refs=$(grep -o '#[0-9][0-9]*' "$body_file" | sort -u | head -20 || true)
for ref in $refs; do
  number=${ref#\#}
  gh api "repos/{owner}/{repo}/issues/${number}" --jq '.number' >/dev/null 2>&1 ||
    die "the description references ${ref}, which is not an issue or pull request here."
done
printf 'labels, milestone, and references all resolve\n'

# --- the push, planned -----------------------------------------------
#
# plan_push reads what origin holds now, from origin itself rather than
# from a remote-tracking ref that may be stale, and sets `was` and
# `push_kind`: new, current, fast-forward, or rewrite. It refuses a push
# that is neither a fast-forward nor a rewrite --rewrite allows.

plan_push() {
  was=$(git ls-remote origin "refs/heads/$branch" | cut -f1) ||
    die "could not read ${branch} from origin; nothing was pushed."
  if [ -z "$was" ]; then
    push_kind=new
  elif [ "$was" = "$head" ]; then
    push_kind=current
  elif gitr merge-base --is-ancestor "$was" "$head" 2>/dev/null; then
    push_kind=fast-forward
  elif ! gitr cat-file -e "$was^{commit}" 2>/dev/null; then
    die "origin has ${branch} at ${was}, a commit this checkout never fetched.
Nothing was pushed. Fetch it and look at what arrived before publishing."
  elif gitr merge-base --is-ancestor "$head" "$was"; then
    die "HEAD is behind origin's ${branch} at ${was}, and pushing would drop its
commits. Nothing was pushed. Bring them in first."
  elif [ "$rewrite" = true ]; then
    push_kind=rewrite
  elif [ "$dry_run" = true ]; then
    push_kind=non-fast-forward
  else
    die "origin has ${branch} at ${was}, which HEAD does not contain, so the push
is not a fast-forward and nothing was pushed. Where the branch was rebased
on purpose, run this again with --rewrite on the operator's word: it
replaces only that exact commit. Otherwise merge the base or the remote
commits into the branch rather than rebasing it."
  fi
}

step "planning the push"
plan_push
case $push_kind in
new) printf 'origin has no %s yet; the push creates it\n' "$branch" ;;
current) printf 'origin already has %s at %s\n' "$branch" "$head" ;;
fast-forward) printf 'fast-forward from %s\n' "$was" ;;
rewrite) printf 'REWRITE: replaces origin %s at %s, leasing on that exact commit\n' "$branch" "$was" ;;
non-fast-forward)
  printf 'WARNING: origin has %s at %s, which HEAD does not contain. The real run\n' "$branch" "$was"
  printf 'refuses this push unless --rewrite is passed on the operator'"'"'s word.\n'
  ;;
esac

# gh acts as whoever is signed in. A grant to publish says nothing about
# whose account it publishes under, so before any write the signed-in
# login has to be the configured agent account. gh_login is machine-level
# configuration, never a repository file.
check_identity() {
  local want login
  want=$(dt_conf gh_login)
  if [ -z "$want" ]; then
    printf 'gh identity: gh_login is not configured, so the account check is skipped\n'
    return 0
  fi
  login=$(gh api user --jq .login 2>/dev/null) || die "gh could not say which account it is signed in as"
  [ "$login" = "$want" ] ||
    die "gh is signed in as ${login:-nobody}, not ${want}. Nothing was pushed or published."
  printf 'gh identity: %s\n' "$login"
}

if [ "$dry_run" = true ]; then
  step "dry run"
  check_identity
  printf 'dry run: nothing confirmed, pushed, or published\n'
  rm -f "$copy" "$body_file"
  exit 0
fi

# --- confirmation ----------------------------------------------------

step "confirming"
status=0
dt_confirm pr "$confirmed" || status=$?
case $status in
0) ;;
1) exit 1 ;;
*) exit "$DT_HELD" ;;
esac

# The confirmation may have run another program. Check the stamp again,
# then the copy against it, and only then write anything.
require_reviewed
[ "$(dt_digest "$copy")" = "$(dt_stamp_value pr draft)" ] ||
  die "the copy of PR_AGENTDESC.md no longer matches the review stamp. Review it again."
check_identity

# --- push ------------------------------------------------------------

step "pushing ${branch}"
plan_push
case $push_kind in
current) printf 'nothing to push\n' ;;
rewrite)
  git push --set-upstream --force-with-lease="refs/heads/$branch:$was" origin "HEAD:refs/heads/$branch" ||
    die "the rewrite push failed; origin may have moved past ${was}. Nothing was published."
  printf 'REWRITE: %s on origin went from %s to %s\n' "$branch" "$was" "$head"
  ;;
*)
  git push --set-upstream origin "HEAD:refs/heads/$branch" ||
    die "git push failed; nothing was published."
  ;;
esac

# A pre-push hook, or a server that rewrites a ref, could leave something
# other than the reviewed commit on the branch.
[ "$(dt_head)" = "$head" ] ||
  die "HEAD moved during the push. No pull request was touched; review again."
remote=$(git ls-remote origin "refs/heads/$branch" | cut -f1)
[ "$remote" = "$head" ] ||
  die "origin has ${remote:-no ${branch}} after the push, not the reviewed ${head}. No pull request was touched."

# --- publish ---------------------------------------------------------

open_number=$(gh pr list --head "$branch" --state open --json number --jq '.[0].number // empty') ||
  die "gh could not list pull requests for ${branch}; the branch is pushed, so run this again"

if [ -n "$open_number" ]; then
  step "updating pull request #${open_number}"
  current=$(gh pr view "$open_number" \
    --json baseRefName,isDraft,labels,reviewRequests,assignees,milestone)
  gh pr edit "$open_number" --base "$base" --title "$title" --body-file "$body_file" >/dev/null

  # reconcile <add-flag> <remove-flag> <desired> <current>: bring one
  # property set to what the draft says, removals included.
  reconcile() {
    local item
    while IFS= read -r item; do
      [ -n "$item" ] || continue
      printf '%s\n' "$3" | grep -qxF -- "$item" ||
        gh pr edit "$open_number" "$2" "$item" >/dev/null
    done <<EOF
$4
EOF
    while IFS= read -r item; do
      [ -n "$item" ] || continue
      printf '%s\n' "$4" | grep -qxF -- "$item" ||
        gh pr edit "$open_number" "$1" "$item" >/dev/null
    done <<EOF
$3
EOF
  }

  reconcile --add-label --remove-label "$labels" \
    "$(printf '%s' "$current" | jq -r '.labels[].name')"
  reconcile --add-reviewer --remove-reviewer "$reviewers" \
    "$(printf '%s' "$current" | jq -r '.reviewRequests[].login // empty')"
  reconcile --add-assignee --remove-assignee "$assignees" \
    "$(printf '%s' "$current" | jq -r '.assignees[].login')"

  if [ -n "$milestone" ]; then
    gh pr edit "$open_number" --milestone "$milestone" >/dev/null
  elif [ -n "$(printf '%s' "$current" | jq -r '.milestone.title // empty')" ]; then
    gh pr edit "$open_number" --remove-milestone >/dev/null
  fi
  current_draft=$(printf '%s' "$current" | jq -r '.isDraft')
  if [ "$is_draft" = true ] && [ "$current_draft" = false ]; then
    gh pr ready "$open_number" --undo >/dev/null
  elif [ "$is_draft" = false ] && [ "$current_draft" = true ]; then
    gh pr ready "$open_number" >/dev/null
  fi
  gh pr view "$open_number" --json url --jq '.url'
  rm -f "$copy" "$body_file"
  step "done"
  printf 'description updated at %s. PR_AGENTDESC.md stays for the next edit.\n' "$head"
  exit 0
fi

step "opening the pull request"
set -- --base "$base" --head "$branch" --title "$title" --body-file "$body_file"
if [ "$is_draft" = true ]; then
  set -- "$@" --draft
fi
while IFS= read -r item; do
  [ -n "$item" ] || continue
  set -- "$@" --label "$item"
done <<EOF
$labels
EOF
while IFS= read -r item; do
  [ -n "$item" ] || continue
  set -- "$@" --reviewer "$item"
done <<EOF
$reviewers
EOF
while IFS= read -r item; do
  [ -n "$item" ] || continue
  set -- "$@" --assignee "$item"
done <<EOF
$assignees
EOF
if [ -n "$milestone" ]; then
  set -- "$@" --milestone "$milestone"
fi

gh pr create "$@" || die "gh pr create failed; the branch is pushed, so run this again"
rm -f "$copy" "$body_file"

step "done"
printf 'pull request open at %s. PR_AGENTDESC.md stays until merge-pr lands the branch.\n' "$head"
