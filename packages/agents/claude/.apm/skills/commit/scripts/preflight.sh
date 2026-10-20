#!/usr/bin/env bash
# preflight — gather every fact the commit skill needs before it stages
# or drafts anything.
#
# Claude inlines this through the !`...` preprocessor and Codex runs it
# as the workflow's first command, so either way it runs once before the
# agent stages anything. That ordering is the point: the agent starts
# from a settled picture of the worktree instead of spending its first
# several tool calls rediscovering it, and the rebase-base decision that
# opens the workflow is already computed.
#
# Output is plain text under `== section ==` headers, each self-contained
# so a partially-read report still answers something. Nothing here
# touches the worktree, the index, or the branch. The one write is the
# guard mark in the git directory, which arms Claude's commit guard for
# this run and is inert under Codex. The one network call fetches the
# base branch; COMMIT_PREFLIGHT_FETCH=0 skips it.
#
# A repository with no commit yet, or no origin/<default>, is a normal
# starting point rather than an error. The first commit has no base to
# rebase onto, so the report says so and the workflow skips that step.
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

# Diff budget. The full patch is worth inlining when it is small enough
# to read, and actively harmful when it is not: an oversized paste
# crowds out the rest of the skill. Past the cap the report prints the
# per-file stat and tells the agent which paths to diff by hand.
readonly DIFF_LINE_CAP=${COMMIT_PREFLIGHT_DIFF_LINES:-600}
readonly DO_FETCH=${COMMIT_PREFLIGHT_FETCH:-1}
readonly FETCH_TIMEOUT=${COMMIT_PREFLIGHT_FETCH_TIMEOUT:-10}

# Commit-message bounds, mirrored from the shared commitlint hook so the
# agent drafts against the numbers that will actually judge it. The hook
# reads the same environment overrides, so a repo that retunes one gets a
# report that matches.
readonly HEADER_MIN=${COMMITLINT_HEADER_MIN_LENGTH:-10}
readonly HEADER_MAX=${COMMITLINT_HEADER_MAX_LENGTH:-80}
readonly BODY_MAX=${COMMITLINT_BODY_MAX_LINE_LENGTH:-72}
readonly FOOTER_MAX=${COMMITLINT_FOOTER_MAX_LINE_LENGTH:-100}
readonly TYPES=${COMMITLINT_TYPES:-"feat fix docs style refactor perf test build ci chore revert"}

section() { printf '\n== %s ==\n' "$1"; }

command -v commit-lint >/dev/null 2>&1 || {
  printf 'commit preflight: commit-lint is not installed; add "go:github.com/tbhb-dev/devtools/cmd/commit-lint" = "<devtools-version>" to mise.toml [tools]\n' >&2
  exit 1
}

# none prints a placeholder when a listing came back empty, so a section
# never reads as truncated output.
none() { printf '(none)\n'; }

# run_with_timeout bounds a command that touches the network. macOS ships
# no coreutils `timeout`, so this uses perl's alarm, which is present on
# every macOS and CI image this repo targets. Without perl the command
# runs unbounded rather than not at all.
run_with_timeout() {
  local secs=$1
  shift
  if command -v perl >/dev/null 2>&1; then
    perl -e 'alarm shift; exec @ARGV' "$secs" "$@"
  else
    "$@"
  fi
}

# ahead_behind prints "ahead N, behind M" for $1 relative to $2, or a
# marker when either ref is missing.
ahead_behind() {
  local left=$1 right=$2 counts
  if ! git rev-parse --verify --quiet "$left" >/dev/null ||
    ! git rev-parse --verify --quiet "$right" >/dev/null; then
    printf 'unknown (missing ref)\n'
    return
  fi
  counts=$(git rev-list --left-right --count "$right...$left")
  printf 'ahead %s, behind %s\n' "${counts##*[[:space:]]}" "${counts%%[[:space:]]*}"
}

# behind_count prints just the behind count of $1 relative to $2, for the
# arithmetic the base decision needs.
behind_count() {
  local counts
  counts=$(git rev-list --left-right --count "$2...$1" 2>/dev/null) || {
    printf '0\n'
    return
  }
  printf '%s\n' "${counts%%[[:space:]]*}"
}

root=$(git rev-parse --show-toplevel)
cd "$root"
if [ "${DT_UNATTENDED:-0}" = 1 ]; then
  dt_agent_context
  dt_agent_signing
fi
# symbolic-ref rather than rev-parse: before the first commit HEAD names
# a branch that does not exist yet, and rev-parse fails on it.
branch=$(git symbolic-ref --quiet --short HEAD || printf 'detached\n')
head=$(dt_head)

# Default branch, preferring what the remote actually points HEAD at
# over a guess. A worktree cloned without the symref falls back to a
# local main, then master.
default_branch=main
if remote_head=$(git symbolic-ref --quiet refs/remotes/origin/HEAD 2>/dev/null); then
  default_branch=${remote_head##*/}
elif ! git rev-parse --verify --quiet refs/heads/main >/dev/null &&
  git rev-parse --verify --quiet refs/heads/master >/dev/null; then
  default_branch=master
fi

section "worktree"
printf 'root:     %s\n' "$root"
printf 'branch:   %s\n' "$branch"
case $root in
*/.claude/worktrees/*) printf 'layout:   agent worktree under .claude/worktrees\n' ;;
*/.codex/worktrees/*) printf 'layout:   agent worktree under .codex/worktrees\n' ;;
*) printf 'layout:   primary checkout\n' ;;
esac
upstream=$(git rev-parse --abbrev-ref --symbolic-full-name '@{upstream}' 2>/dev/null || echo none)
printf 'upstream: %s\n' "$upstream"

# An interrupted rebase, merge, or cherry-pick makes every downstream
# step unsafe, so surface it before the agent stages anything.
git_dir=$(git rev-parse --git-dir)
in_progress=none
if [ -d "$git_dir/rebase-merge" ] || [ -d "$git_dir/rebase-apply" ]; then
  in_progress="rebase (finish or abort it first)"
elif [ -f "$git_dir/MERGE_HEAD" ]; then
  in_progress="merge (finish or abort it first)"
elif [ -f "$git_dir/CHERRY_PICK_HEAD" ]; then
  in_progress="cherry-pick (finish or abort it first)"
fi
printf 'in progress: %s\n' "$in_progress"

# Arm the commit guard for this run. Under Claude that guard is a
# skill-frontmatter hook, so it stays registered for the rest of the
# session and needs a signal telling it a workflow is actually open.
# Codex has no such hook and ignores the mark. HEAD is that signal:
# the workflow leaves HEAD alone until it commits, and the commit that
# ends the workflow moves HEAD past this mark and stands the guard down
# without anyone having to clear it.
printf '%s' "$head" >"$(git rev-parse --absolute-git-dir)/commit-workflow.head"
printf 'commit guard: armed at %s, inert again once HEAD moves\n' "$head"

section "rebase base"
fetch_state=skipped
if [ "$DO_FETCH" = "1" ]; then
  if run_with_timeout "$FETCH_TIMEOUT" git fetch --quiet origin "$default_branch" 2>/dev/null; then
    fetch_state=ok
  else
    fetch_state="failed (origin counts below may be stale)"
  fi
fi
printf 'fetch:  %s\n' "$fetch_state"
printf 'default branch: %s\n' "$default_branch"
# shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
printf '%s vs origin/%s:  %s\n' "$default_branch" "$default_branch" \
  "$(ahead_behind "refs/heads/$default_branch" "refs/remotes/origin/$default_branch")"
# shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
printf '%s vs %s:  %s\n' "$branch" "$default_branch" \
  "$(ahead_behind HEAD "refs/heads/$default_branch")"
# shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
printf '%s vs origin/%s:  %s\n' "$branch" "$default_branch" \
  "$(ahead_behind HEAD "refs/remotes/origin/$default_branch")"

# Local main is only a safe rebase target when it carries everything
# origin has. One commit behind and rebasing onto it replays the branch
# over a stale base, which shows up later as a needless second rebase.
#
# A root commit has nothing to rebase onto, and neither does a branch
# whose default exists nowhere yet. Both skip the rebase step.
has_local=false has_origin=false
git rev-parse --verify --quiet "refs/heads/$default_branch" >/dev/null && has_local=true
git rev-parse --verify --quiet "refs/remotes/origin/$default_branch" >/dev/null && has_origin=true
if [ "$head" = unborn ]; then
  printf 'recommended base: none (root commit)\n'
  printf 'reason: HEAD has no commit yet, so this is the first commit; skip the rebase step\n'
elif [ "$has_origin" = false ] && [ "$has_local" = false ]; then
  printf 'recommended base: none\n'
  printf 'reason: neither %s nor origin/%s exists; skip the rebase step\n' "$default_branch" "$default_branch"
elif [ "$has_origin" = false ]; then
  printf 'recommended base: %s\n' "$default_branch"
  printf 'reason: origin/%s does not exist, so local %s is the only base\n' "$default_branch" "$default_branch"
else
  local_behind=$(behind_count "refs/heads/$default_branch" "refs/remotes/origin/$default_branch")
  if [ "$has_local" = true ] && [ "$local_behind" = "0" ]; then
    printf 'recommended base: %s\n' "$default_branch"
    printf 'reason: local %s carries everything origin/%s has\n' "$default_branch" "$default_branch"
  elif [ "$has_local" = true ]; then
    printf 'recommended base: origin/%s\n' "$default_branch"
    printf 'reason: local %s is %s commit(s) behind origin/%s\n' \
      "$default_branch" "$local_behind" "$default_branch"
  else
    printf 'recommended base: origin/%s\n' "$default_branch"
    printf 'reason: there is no local %s\n' "$default_branch"
  fi
fi

section "preconditions"
if git check-ignore --quiet COMMIT_AGENTMSG 2>/dev/null; then
  printf 'COMMIT_AGENTMSG gitignored: yes\n'
else
  printf 'COMMIT_AGENTMSG gitignored: NO — add it to .gitignore before drafting\n'
fi
if [ "${DT_UNATTENDED:-0}" = 1 ]; then
  lint_cmd=$(dt_repo_conf lint_commit_cmd 'mise run lint-commit-msg')
  [ -n "$lint_cmd" ] || dt_die "lint_commit_cmd must not be empty"
  printf 'commit message gate: %s\n' "$lint_cmd"
elif command -v mise >/dev/null 2>&1 && mise task info lint-commit-msg >/dev/null 2>&1; then
  printf 'mise run lint-commit-msg: present\n'
else
  printf 'mise run lint-commit-msg: ABSENT — stop and tell the operator\n'
fi

# prek installs shims into git's effective hooks directory, which
# core.hooksPath overrides when set.
hooks_dir=$(git config --get core.hooksPath || true)
[ -n "$hooks_dir" ] || hooks_dir="$(git rev-parse --git-common-dir)/hooks"
for hook in commit-msg pre-commit post-commit; do
  if [ -x "$hooks_dir/$hook" ]; then
    printf '%s hook: installed\n' "$hook"
  else
    printf '%s hook: not installed — run mise run devtools:prek-install\n' "$hook"
  fi
done
if [ -s COMMIT_AGENTMSG ]; then
  # shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
  printf 'COMMIT_AGENTMSG: present, %s line(s) — a leftover draft; overwrite it\n' \
    "$(wc -l <COMMIT_AGENTMSG | tr -d ' ')"
else
  printf 'COMMIT_AGENTMSG: absent or empty (expected before drafting)\n'
fi

# Who can answer step 8 besides the operator. commit.sh resolves this
# itself at commit time, in the same order; the report here is so the
# agent knows before it gets there whether to ask. A grant comes from
# `mise run preapprove`, keyed on the session. A delegation comes from
# the machine's delegation_check, which this only reports as configured
# rather than calling, because asking it is the commit's job.
section "pre-approval"
session=$(dt_session_id)
if [ -n "$session" ] && grant=$(dt_preapproval commit); then
  case ${grant##* } in
  touchid) how="a Touch ID prompt stands behind it" ;;
  *) how="no biometric prompt stood behind it" ;;
  esac
  printf 'commit: GRANTED, %s — step 8 skips the confirmation once every gate and the review pass\n' "$how"
elif [ "${DT_UNATTENDED:-0}" = 1 ]; then
  printf 'commit: not granted; delegation must authorize the write, otherwise report the hold\n'
else
  printf 'commit: not granted — step 8 confirms with the operator as written\n'
fi
checker=$(dt_conf delegation_check)
if [ -n "$checker" ]; then
  printf 'delegation: %s is configured; commit.sh asks it when nobody else has answered\n' "$checker"
else
  printf 'delegation: none configured\n'
fi

section "review"
# shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
printf 'harness:  %s\n' "$(dt_harness)"
# shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
printf 'reviewer: %s\n' "$(dt_reviewer_label)"
git_dir=$(dt_git_dir) || exit $?
if [ -f "$git_dir/agent-skills/commit.stamp" ]; then
  printf 'stamp:    present from an earlier review; review.sh replaces it\n'
else
  printf 'stamp:    none\n'
fi

section "staged changes"
staged=$(gitr diff --no-ext-diff --cached --name-status)
if [ -n "$staged" ]; then printf '%s\n' "$staged"; else none; fi

section "unstaged changes"
unstaged=$(gitr diff --no-ext-diff --name-status)
if [ -n "$unstaged" ]; then printf '%s\n' "$unstaged"; else none; fi

section "untracked files"
untracked=$(git ls-files --others --exclude-standard)
if [ -n "$untracked" ]; then printf '%s\n' "$untracked"; else none; fi

section "diffstat"
printf 'staged:\n'
gitr diff --no-ext-diff --cached --stat || true
printf '\nunstaged:\n'
gitr diff --no-ext-diff --stat || true

# The patch itself, tracked changes only. Untracked file bodies stay out:
# they are listed above, and a new vendored file would swamp the report.
section "diff"
patch=$(
  gitr diff --no-ext-diff --cached
  gitr diff --no-ext-diff
)
# shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
if [ -z "$patch" ]; then
  printf '(no tracked changes)\n'
elif [ "$(printf '%s\n' "$patch" | wc -l)" -le "$DIFF_LINE_CAP" ]; then
  printf '%s\n' "$patch"
else
  printf '(diff exceeds %s lines. Read it one path at a time with gitr diff --no-ext-diff\n' "$DIFF_LINE_CAP"
  printf 'and gitr diff --no-ext-diff --cached before grouping the commits.)\n'
fi

section "recent commits"
printf 'subjects:\n'
gitr log --oneline -10 2>/dev/null || printf '(no history)\n'
printf '\nlast two messages in full, for project style:\n'
gitr log -2 --pretty=format:'--- %h%n%B' 2>/dev/null || true

section "message rules"
printf 'types:   %s\n' "$TYPES"
printf 'subject: %s to %s chars, imperative mood, no trailing period\n' "$HEADER_MIN" "$HEADER_MAX"
printf 'body:    hard wrap at %s chars\n' "$BODY_MAX"
printf 'footer:  wrap at %s chars\n' "$FOOTER_MAX"
printf 'trailers: Assisted-by before Signed-off-by; Signed-off-by required\n'
signoff_identity=$(dt_signoff_identity) || exit $?
printf 'Signed-off-by: %s\n' "$signoff_identity"
