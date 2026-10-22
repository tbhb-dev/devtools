#!/usr/bin/env bash
# preflight — gather every fact the pr skill needs before it drafts a
# description or opens anything.
#
# The Claude pr SKILL.md inlines this through the !`...` preprocessor,
# so it runs once before the agent reads the skill body; the Codex one
# runs it as the first step. The report is meant
# to be the last word on the branch: the commits that would land, their
# messages in full, the files they touch, the repository's label set,
# whether a pull request already exists, and the exact frontmatter to
# copy. An agent that has read this should not need to run git log, gh
# pr list, or gh label list to draft.
#
# Output is plain text under `== section ==` headers, each self-contained
# so a partially-read report still answers something. Two network calls
# happen: a fetch of the base branch, which PR_PREFLIGHT_FETCH=0 skips,
# and the pull request lookup the stale-draft rule below depends on.
#
# The one thing this mutates is a stale PR_AGENTDESC.md. The draft is the
# working copy of the description while a pull request is open, so it
# survives create-pr and merge-pr removes it at the end. A draft with no
# open pull request behind it is therefore a leftover from a run that
# stopped early, and leaving it in place is worse than removing it: the
# next run would read someone else's abandoned text as its own starting
# point. Removed rather than emptied, so a stale read of it cannot
# survive either.
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

# lib.sh brings gitr, which runs git for output this script parses with
# every formatting knob pinned, and the session, configuration, and
# review helpers the sections below report on. Plain `git` stays
# available on purpose: config reads and fetch need the operator's real
# configuration.
# shellcheck disable=SC1091 # lib.sh is generated beside this script
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
# shellcheck disable=SC1091 # generated beside preflight
. "$DT_LIB_DIR/description.sh"
# shellcheck disable=SC1091 # unattended entry point shares preflight
. "$DT_LIB_DIR/pr-agent.sh"

# Budgets. A branch carrying a hundred commits is worth summarizing
# rather than transcribing, and an oversized paste crowds out the rest
# of the skill.
readonly LOG_CAP=${PR_PREFLIGHT_LOG_COMMITS:-20}
readonly DIFF_LINE_CAP=${PR_PREFLIGHT_DIFF_LINES:-400}
readonly ISSUE_CAP=${PR_PREFLIGHT_ISSUES:-15}
readonly DO_FETCH=${PR_PREFLIGHT_FETCH:-1}
readonly FETCH_TIMEOUT=${PR_PREFLIGHT_FETCH_TIMEOUT:-10}
readonly DRAFT=PR_AGENTDESC.md

section() { printf '\n== %s ==\n' "$1"; }
none() { printf '(none)\n'; }

# run_with_timeout bounds a command that touches the network. macOS
# ships no coreutils `timeout`, so this uses perl's alarm, which is
# present on every macOS and CI image this repo targets.
run_with_timeout() {
  local secs=$1
  shift
  if command -v perl >/dev/null 2>&1; then
    perl -e 'alarm shift; exec @ARGV' "$secs" "$@"
  else
    "$@"
  fi
}

# ahead_behind prints "ahead N, behind M" for $1 relative to $2.
ahead_behind() {
  local counts
  if ! git rev-parse --verify --quiet "$1" >/dev/null ||
    ! git rev-parse --verify --quiet "$2" >/dev/null; then
    printf 'unknown (missing ref)\n'
    return
  fi
  counts=$(git rev-list --left-right --count "$2...$1")
  printf 'ahead %s, behind %s\n' "${counts##*[[:space:]]}" "${counts%%[[:space:]]*}"
}

root=$(git rev-parse --show-toplevel)
cd "$root"
bash "$(dirname "${BASH_SOURCE[0]}")/worktree-ready.sh"
pr_agent_context
branch=$(git rev-parse --abbrev-ref HEAD)

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

if [ "$branch" = "$default_branch" ]; then
  printf 'WARNING: this is the default branch. A pull request needs a branch of its own.\n'
fi

section "base and divergence"
fetch_state=skipped
if [ "$DO_FETCH" = "1" ]; then
  if run_with_timeout "$FETCH_TIMEOUT" git fetch --quiet origin "$default_branch" 2>/dev/null; then
    fetch_state=ok
  else
    fetch_state="failed (counts below may be stale)"
  fi
fi
printf 'fetch:  %s\n' "$fetch_state"
printf 'default branch: %s\n' "$default_branch"
printf 'recommended base: %s\n' "$default_branch"

base_ref="refs/remotes/origin/$default_branch"
if ! git rev-parse --verify --quiet "$base_ref" >/dev/null; then
  base_ref="refs/heads/$default_branch"
fi
pr_agent_range "$base_ref"
printf 'comparing against: %s\n' "$base_ref"
# shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
printf '%s vs base:  %s\n' "$branch" "$(ahead_behind HEAD "$base_ref")"
merge_base=$(git merge-base HEAD "$base_ref" 2>/dev/null || echo unknown)
printf 'merge base: %s\n' "$merge_base"

section "commits that would land"
landing=$(git rev-list --count "$base_ref..HEAD" 2>/dev/null || echo 0)
printf 'count: %s\n\n' "$landing"
if [ "$landing" = "0" ]; then
  printf 'Nothing to open a pull request for. Commit the work first.\n'
else
  gitr log --oneline "$base_ref..HEAD" | head -n "$LOG_CAP"
  printf '\nmessages in full:\n'
  gitr log --reverse --pretty=format:'--- %h%n%B' "$base_ref..HEAD" | head -n 200
fi

section "commit types on the branch"
if [ "$landing" != "0" ]; then
  gitr log --format='%s' "$base_ref..HEAD" |
    sed -n 's/^\([a-z][a-z]*\)[(!:].*/\1/p' | sort | uniq -c | sort -rn
  printf '\nThe pull request title takes one of these types. A squash merge\n'
  printf 'turns that title into the commit subject on the default branch.\n'
else
  none
fi

section "changed files"
if [ "$landing" != "0" ]; then
  gitr diff --no-ext-diff --name-status "$base_ref...HEAD"
  printf '\n'
  gitr diff --no-ext-diff --stat "$base_ref...HEAD"
else
  none
fi

section "diff"
if [ "$landing" != "0" ]; then
  patch=$(gitr diff --no-ext-diff "$base_ref...HEAD")
  # shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
  if [ "$(printf '%s\n' "$patch" | wc -l)" -le "$DIFF_LINE_CAP" ]; then
    printf '%s\n' "$patch"
  else
    printf '(diff exceeds %s lines. Read it one path at a time with\n' "$DIFF_LINE_CAP"
    printf 'gitr diff --no-ext-diff %s...HEAD -- <path> before drafting.)\n' "$base_ref"
  fi
else
  none
fi

section "push state"
dirty=$(git status --porcelain)
if [ -n "$dirty" ]; then
  printf 'uncommitted changes: yes — these will NOT be in the pull request\n'
  printf '%s\n' "$dirty"
else
  printf 'uncommitted changes: none\n'
fi
if [ "$upstream" = none ]; then
  printf 'pushed: no upstream yet; creating the pull request pushes the branch\n'
else
  printf 'upstream:  %s\n' "$upstream"
  # shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
  printf 'vs upstream: %s\n' "$(ahead_behind HEAD "$upstream")"
fi

section "existing pull request"
gh_state=ok
if ! command -v gh >/dev/null 2>&1; then
  gh_state="gh is not installed"
else
  dt_gh_login >/dev/null || exit $?
fi
open_number=""
if [ "$gh_state" != ok ]; then
  printf '%s\n' "$gh_state"
else
  existing=$(dt_gh pr list --head "$branch" --state all \
    --json number,state,isDraft,url,title \
    --template '{{range .}}#{{.number}} {{.state}}{{if .isDraft}} (draft){{end}} {{.title}}
{{.url}}
{{end}}') || exit $?
  open_number=$(dt_gh pr list --head "$branch" --state open --json number \
    --jq '.[0].number') || exit $?
  if [ -n "$existing" ]; then
    printf '%s\n' "$existing"
    if [ -n "$open_number" ]; then
      printf 'One is open. Re-running create-pr.sh updates it rather than opening\n'
      printf 'a second one.\n'
    fi
  else
    printf 'none for %s\n' "$branch"
  fi
fi

section "repository"
if [ "$gh_state" = ok ]; then
  dt_gh repo view --json nameWithOwner,defaultBranchRef \
    --template 'repo: {{.nameWithOwner}}
default branch: {{.defaultBranchRef.name}}
' || exit $?
  printf '\nlabels:\n'
  dt_gh label list --limit 100 || exit $?
  printf '\nopen milestones:\n'
  milestones=$(dt_gh_repo "query(\$owner:String!,\$name:String!){repository(owner:\$owner,name:\$name){milestones(first:100,states:OPEN){nodes{title} pageInfo{hasNextPage}}}}" --jq '.data.repository.milestones | if .pageInfo.hasNextPage then error("more than 100 open milestones") else .nodes[].title end') || exit $?
  if [ -n "$milestones" ]; then printf '%s\n' "$milestones"; else none; fi
  printf '\nopen issues (newest first, for the Related section):\n'
  issues=$(dt_gh issue list --state open --limit "$ISSUE_CAP" \
    --json number,title --template '{{range .}}#{{.number}} {{.title}}
{{end}}') || exit $?
  if [ -n "$issues" ]; then printf '%s\n' "$issues"; else none; fi
else
  printf '%s\n' "$gh_state"
fi

section "template sections"
template=$(pr_template)
if [ -f "$template" ]; then
  printf 'from %s, in this order:\n' "$template"
  grep '^## ' "$template" || printf '(the template declares no level 2 headings)\n'
else
  printf 'NO TEMPLATE at %s — stop and tell the operator\n' "$template"
fi

# Who can answer the publish besides the operator, and how far a
# pre-approved run goes. create-pr.sh resolves the confirmation itself
# at publish time, in the same order; this report is so the agent knows
# before it gets there whether to ask. A grant comes from `mise run
# preapprove`, keyed on the session. A delegation comes from the
# machine's delegation_check, which this only reports as configured
# rather than calling, because asking it is the publish's job.
#
# The merge line matters here as well as in merge-pr, because the
# confirmation step routes on how far to take the branch, and a grant
# that stops short of merging routes to watching and fixing rather than
# to the merge.
section "pre-approval"
how() {
  case ${1##* } in
  touchid) printf 'a Touch ID prompt stands behind it' ;;
  *) printf 'no biometric prompt stood behind it' ;;
  esac
}
if grant=$(dt_preapproval pr); then
  # shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
  printf 'pr:    GRANTED, %s — the confirmation step is skipped once every gate and the review pass\n' "$(how "$grant")"
else
  if [ "${DEVTOOLS_UNATTENDED:-0}" = 1 ]; then
    printf 'pr: not granted — delegation must authorize publication; otherwise report the hold without prompting\n'
  else
    printf 'pr:    not granted — the confirmation step asks the operator as written\n'
  fi
fi
if grant=$(dt_preapproval merge); then
  # shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
  printf 'merge: GRANTED, %s — a pre-approved run routes through to the squash merge\n' "$(how "$grant")"
else
  printf 'merge: not granted — a pre-approved run stops once the checks are green\n'
fi
checker=$(dt_conf delegation_check)
if [ -n "$checker" ]; then
  printf 'delegation: %s is configured; create-pr.sh asks it when nobody else has answered\n' "$checker"
else
  printf 'delegation: none configured\n'
fi
gh_login=$(dt_conf gh_login)
if [ -n "$gh_login" ]; then
  printf 'gh identity: create-pr.sh publishes only while gh is signed in as %s\n' "$gh_login"
else
  printf 'gh identity: gh_login is not configured, so create-pr.sh skips the account check\n'
fi

section "review"
# shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
printf 'harness:  %s\n' "$(dt_harness)"
# shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
printf 'reviewer: %s\n' "$(dt_reviewer_label)"
state=$(dt_state_dir) || exit $?
if [ -f "$state/pr.stamp" ]; then
  printf 'stamp:    present from an earlier review; create-pr.sh checks it is still current\n'
else
  printf 'stamp:    none\n'
fi

section "preconditions"
dt_require_gh_post || exit $?
if git check-ignore --quiet "$DRAFT" 2>/dev/null; then
  printf '%s gitignored: yes\n' "$DRAFT"
else
  printf '%s gitignored: NO — add it to .gitignore before drafting\n' "$DRAFT"
fi
if [ "${DEVTOOLS_UNATTENDED:-0}" = 1 ]; then
  lint_cmd=$(dt_repo_conf lint_pr_cmd 'mise run lint-pr-description')
  [ -n "$lint_cmd" ] || dt_die "lint_pr_cmd must not be empty"
  printf 'description lint: %s\n' "$lint_cmd"
elif command -v mise >/dev/null 2>&1 && mise task info lint-pr-description >/dev/null 2>&1; then
  printf 'mise run lint-pr-description: present\n'
else
  printf 'mise run lint-pr-description: ABSENT — stop and tell the operator\n'
fi
for skill in pr review-pr-description; do
  if [ -f "$DT_LIB_DIR/../../$skill/SKILL.md" ]; then
    printf '%s skill deployed: yes\n' "$skill"
  else
    printf '%s skill deployed: NO — run apm install\n' "$skill"
  fi
done
if [ ! -s "$DRAFT" ]; then
  printf '%s: absent or empty (expected before drafting)\n' "$DRAFT"
elif [ "$gh_state" != ok ]; then
  # shellcheck disable=SC2312 # Only formats diagnostic output; no decision uses the status.
  printf '%s: present, %s line(s). Cannot tell whether it is stale because\n' \
    "$DRAFT" "$(wc -l <"$DRAFT" | tr -d ' ')"
  printf 'the pull request lookup did not run. Read it before overwriting.\n'
elif [ -z "$open_number" ] && [ "${DEVTOOLS_UNATTENDED:-0}" = 1 ]; then
  printf '%s: present, unpublished draft preserved with its review bound\n' "$DRAFT"
elif [ -z "$open_number" ]; then
  rm -f "$DRAFT"
  dt_clear pr
  printf '%s: REMOVED. It carried no open pull request, so it was a leftover\n' "$DRAFT"
  printf 'from a run that stopped early.\n'
else
  printf '%s: present, the working copy for #%s\n' "$DRAFT" "$open_number"
  # Whether the draft still matches what is published decides the next
  # step, and finding out otherwise costs the agent two more calls.
  fm_end=$(awk 'NR > 1 && $0 == "---" { print NR; exit }' "$DRAFT" || true)
  title_line=$(awk -v last="${fm_end:-0}" 'NR > last && /^# / { print NR; exit }' "$DRAFT" || true)
  if [ -n "$title_line" ]; then
    drafted=$(awk -v start="$title_line" 'NR > start' "$DRAFT" | awk 'NF { seen = 1 } seen')
    published=$(dt_gh pr view "$open_number" --json body --jq '.body') || exit $?
    if [ "$(printf '%s' "$drafted")" = "$(printf '%s' "$published")" ]; then
      printf 'It matches the published description, so nothing needs republishing.\n'
    else
      printf 'It differs from the published description. Republish with create-pr.sh\n'
      printf 'once the review clears it again.\n'
    fi
  fi
fi

section "draft"
# Scaffolding here rather than in an instruction keeps the sections
# matching the template as it stands, and leaves the drafting session a
# file to fill instead of a shape to reproduce from memory. It runs
# after the sweep above, which decides what should be on disk at all.
scaffold="$DT_LIB_DIR/scaffold-description.sh"
if [ -f "$scaffold" ]; then
  bash "$scaffold" "$default_branch"
else
  printf '(scaffold-description.sh missing; run apm install)\n'
fi
