#!/usr/bin/env bash
# commit — commit the reviewed message, then check that what landed is
# what was staged and reviewed.
#
# `git commit` alone does not settle that. The pre-commit hooks run
# through prek, which stashes and restores the worktree around them, and
# a run ending in failure can hand the index back short of what it was
# given. The retry then commits a subset of the group without saying so,
# and the only trace is a diffstat scrolling past. So the staged set is
# recorded first and the commit is compared against it afterwards.
#
# In order, it refuses unless: the draft and a staged change exist, the
# review stamp covers the draft's bytes, the staged tree, the commit at
# HEAD, and the mode, and somebody confirmed the commit (the operator
# flag, a pre-approval for this session, or the machine's delegation
# check, in that order). The confirmation is another program, so the
# stamp is checked again after it, and the commit is made from this
# script's own copy of the stamped bytes.
#
# After the commit, two checks read it back. Every staged path has to be
# in it, measured against the first parent, so a merge commit is judged
# on what it adds to the branch and a root commit on everything it
# holds. And its tree has to be the tree the reviewer read. A hook that
# rewrites the index and lets the commit through fails the second check;
# the commit stays, and the output says to review and amend it. The usual
# autofixing hook never gets that far: prek fails the commit when a
# fixer changes a file, the fix has to be staged by hand, and the new
# staged tree no longer matches the stamp, so the change goes back
# through review before it can commit at all.
#
# Usage: commit.sh [--amend] [--confirmed]
#   --amend      fold the staged change into the commit at HEAD
#   --confirmed  the operator answered yes to this commit in this session
# Exit:  0 committed and verified
#        1 refused before committing, or committed but failed a check
#        75 held: nobody has confirmed the commit; ask, then pass --confirmed
#        anything else is git's own status from a failed commit
set -euo pipefail

# --- environment hardening -------------------------------------------
# lib.sh repeats this block; it stays here too so the hygiene gate can
# see it in the file that calls git.
export LC_ALL=C
export GH_PAGER=cat
export GH_PROMPT_DISABLED=1
export PYTHONUTF8=1
unset CDPATH GH_REPO GH_HOST GREP_OPTIONS
IFS=$' \t\n'

# shellcheck disable=SC1091 # lib.sh is generated beside this script
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

fail() {
  printf 'Refused by the commit workflow.\n\n%s\n' "$1" >&2
  exit 1
}

# Only --amend and --confirmed pass. Everything else this workflow needs
# is supplied here, and an allowlist keeps a caller from carrying
# --no-verify or another unsafe flag through the wrapper.
mode=normal
confirmed=false
for arg in "$@"; do
  case $arg in
  --amend) mode=amend ;;
  --confirmed) confirmed=true ;;
  *)
    fail "commit.sh takes --amend and --confirmed and nothing else. It supplies the
message file and the cleanup mode itself, and the flags refused elsewhere
in this workflow are refused here for the same reasons.

Received: ${arg}"
    ;;
  esac
done

root=$(dt_root)
cd "$root"
draft=$root/COMMIT_AGENTMSG
state=$(dt_state_dir)
here=$DT_LIB_DIR

[ -s "$draft" ] || fail "COMMIT_AGENTMSG is empty or missing. Draft the message first."
start=$(dt_head)
[ "$mode" = normal ] || [ "$start" != unborn ] || fail "There is no commit to amend yet."

staged() { gitr diff --no-ext-diff --cached --name-only --no-renames | sort; }

manifest=$(staged)
if [ -z "$manifest" ] && [ "$mode" = normal ]; then
  fail "Nothing is staged, so there is nothing to commit.

Stage the paths this commit is about, one at a time:

  git add -- path/one path/two"
fi

# The stamp has to match everything the review read.
require_reviewed() {
  [ -f "$state/commit.stamp" ] || fail "No independent review has cleared this draft.

That review is the only gate on the things linting cannot see: claims the
diff does not support, restating the diff instead of explaining it, and
whether the staged change is really one logical change. Run it:

  bash $here/review.sh$([ "$mode" = amend ] && printf ' --amend')"
  (dt_require_stamp commit "$draft" tree "$(gitr write-tree)" head "$(dt_head)" mode "$mode") ||
    fail "The reviewed change and the change about to be committed are no longer
the same. Run the review again against the current draft and index:

  bash $here/review.sh$([ "$mode" = amend ] && printf ' --amend')"
}
require_reviewed
tree=$(gitr write-tree)

status=0
dt_confirm commit "$confirmed" || status=$?
case $status in
0) ;;
1) exit 1 ;;
*) exit "$DT_HELD" ;;
esac

# The confirmation may have run another program, and anything could
# have rewritten the draft or the index meanwhile. Check again, then
# commit from a copy of the bytes the stamp covers.
require_reviewed
message=$state/commit.message
cp "$draft" "$message"
[ "$(dt_digest "$message")" = "$(dt_stamp_value commit draft)" ] ||
  fail "COMMIT_AGENTMSG changed while it was being copied. Run the review again."

# The commit runs under the operator's own git configuration apart from
# the cleanup mode. Left at its default of strip, git would drop every
# body line opening with a number sign, after the review hashed the
# file, so the bytes the reviewer cleared would stop matching the bytes
# git records.
set -- -c commit.cleanup=whitespace commit -F "$message"
[ "$mode" = normal ] || set -- "$@" --amend

# snapshot prints one line naming the index, the tracked edits in the
# worktree, and the untracked files. Taken before and after the attempt,
# it separates a hook that failed from a hook that changed something: the
# two need different advice, and a failure that changed nothing is not an
# autofixer.
snapshot() {
  {
    gitr write-tree
    gitr diff --no-ext-diff
    gitr status --porcelain --untracked-files=all
  } | gitr hash-object --stdin
}
before=$(snapshot)
status=0
command git "$@" || status=$?

if [ "$status" != 0 ]; then
  dropped=""
  [ -z "$manifest" ] || dropped=$(comm -23 <(printf '%s\n' "$manifest") <(staged))
  if [ -n "$dropped" ]; then
    printf '\nThe commit failed and the index came back short. These paths were\n' >&2
    printf 'staged before the attempt and are not staged now:\n\n' >&2
    printf '%s\n' "$dropped" | sed 's/^/  /' >&2
    printf '\nStage them again before retrying, or the retry commits a subset of\n' >&2
    printf 'the group without saying so.\n' >&2
  fi
  if [ "$(snapshot)" = "$before" ]; then
    printf '\nA hook failed, and nothing in the index or the worktree changed. Its\n' >&2
    printf 'output is above. Fix what it reported; the staged change and the stamp\n' >&2
    printf 'are untouched, so a fix to a staged file means staging it and running\n' >&2
    printf 'review.sh again.\n' >&2
  else
    printf '\nA hook changed files during the attempt, which fails the commit on\n' >&2
    printf 'purpose. Stage the change by name, then run review.sh again: the\n' >&2
    printf 'staged tree changed, so the stamp no longer covers it.\n' >&2
  fi
  exit "$status"
fi

# What the commit actually holds, against what the manifest named. An
# amend carries its original contents too, so the test is coverage
# rather than equality: every path the manifest named has to be there.
#
# The paths are read against the first parent, which is what the index
# was compared against before the commit. diff-tree on a merge commit
# prints nothing without -m, and --first-parent alone does not change
# that, so a merge reads its first parent explicitly. A root commit
# reads against the empty tree through --root.
landed_paths() {
  local parents
  parents=$(gitr rev-list --parents -n 1 HEAD | wc -w)
  if [ "$parents" -gt 2 ]; then
    gitr diff --no-ext-diff --name-only --no-renames 'HEAD^1' HEAD
  else
    gitr diff-tree --no-ext-diff --no-commit-id --name-only --no-renames -r --root HEAD
  fi
}
missing=""
if [ -n "$manifest" ]; then
  missing=$(comm -23 <(printf '%s\n' "$manifest") <(landed_paths | sort))
fi

gitr show --no-ext-diff --stat --format='%h %s' HEAD

failed=false
landed_tree=$(gitr rev-parse 'HEAD^{tree}')
if [ "$landed_tree" != "$tree" ]; then
  printf '\nThe commit tree differs from the staged tree the reviewer cleared, so a\n' >&2
  printf 'hook changed the index during the commit and the commit went through.\n' >&2
  printf 'The commit stays. Compare it with what was reviewed, then stage any\n' >&2
  printf 'correction and amend through this workflow, review included:\n\n' >&2
  printf '  git diff %s HEAD\n' "$tree" >&2
  failed=true
fi

if [ -n "$missing" ]; then
  printf '\nThe commit is short of what was staged. These paths were in the\n' >&2
  printf 'index before the commit and are not in it:\n\n' >&2
  printf '%s\n' "$missing" | sed 's/^/  /' >&2
  printf '\nStage them, review again with --amend, and fold them into the commit\n' >&2
  printf 'that just landed:\n\n' >&2
  printf '  git add -- <the paths above>\n' >&2
  printf '  bash %s/review.sh --amend\n' "$here" >&2
  printf '  bash %s/commit.sh --amend\n' "$here" >&2
  failed=true
fi

dt_clear commit
rm -f "$message"
[ "$failed" = false ] || exit 1
