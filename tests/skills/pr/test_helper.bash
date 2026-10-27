#!/usr/bin/env bash
# Scratch repository and API fixtures shared by both PR entry points.

pr_setup() {
  ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
  source "$ROOT/tests/test_helper.bash"
  isolate_git
  stub_gates
  SCRIPTS="${ROOT}/packages/agents/claude/.apm/skills/pr/scripts"
  REPO="${BATS_TEST_TMPDIR}/repo"
  ORIGIN="${BATS_TEST_TMPDIR}/origin.git"
  unset CLAUDE_CODE_SESSION_ID CODEX_THREAD_ID CLAUDECODE DEVTOOLS_HARNESS
  unset DEVTOOLS_REVIEW_MODEL DEVTOOLS_REVIEW_EFFORT DEVTOOLS_DELEGATION_CHECK DEVTOOLS_GH_LOGIN
  export XDG_CONFIG_HOME="${BATS_TEST_TMPDIR}/config"
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
  export PR_PREFLIGHT_FETCH=0
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo "VERDICT: PASS"'
  export GH_LOG="${BATS_TEST_TMPDIR}/gh.log"
  export GH_LABELS=$'enhancement\nbug' GH_MILESTONES=v1 GH_ISSUES="12" GH_USER=agent GH_OPEN_PR=""
  export GH_PR_STATE='{"baseRefName":"main","isDraft":false,"labels":[{"name":"bug"}],"reviewRequests":[],"assignees":[],"milestone":null}'
  : >"$GH_LOG"
  stub_gh
  stub_size_check

  git init --quiet --bare --initial-branch=main "$ORIGIN"
  git init --quiet --initial-branch=main "$REPO"
  cd "$REPO"
  git config user.name Tester
  git config user.email tester@example.com
  git remote add origin "$ORIGIN"
  mkdir -p .github docs
  printf '## Summary\n\n## Why\n\n## Verification\n' >.github/pull_request_template.md
  printf 'PR_AGENTDESC.md\n' >.gitignore
  printf 'gone\n' >docs/gone.md
  git add -- .github .gitignore docs
  git commit --quiet -m 'chore: initial state'
  git push --quiet origin main
  git checkout --quiet -b topic
  commit_file widget.txt 'feat: add the widget'
  write_draft
}

# gh as a logging stub. Every answer comes from the GH_* variables set
# above, so a case changes one variable to change what GitHub "has".
stub_gh() {
  mkdir -p "${BATS_TEST_TMPDIR}/bin"
  cat >"${BATS_TEST_TMPDIR}/bin/gh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$GH_LOG"
filter=''
previous=''
for arg in "$@"; do
  if [ "$previous" = --jq ]; then filter=$arg; fi
  previous=$arg
done
out() { jq -r "$filter" || return 1; }
case "$1 $2" in
"auth status") exit 0 ;;
"label list") printf '%s\n' "$GH_LABELS" ;;
"api user") echo 'gh: API rate limit exceeded for user ID 1. (HTTP 403)' >&2; exit 1 ;;
"api graphql")
  case "$*" in
  *viewer*)
    if [ -n "${GH_VIEWER_ERROR:-}" ]; then echo "$GH_VIEWER_ERROR" >&2; exit "${GH_ERROR_STATUS:-1}"; fi
    jq -n --arg login "$GH_USER" '{data:{viewer:{login:$login}}}' | out ;;
  *milestones*) printf '%s\n' "$GH_MILESTONES" | jq -Rn '{data:{repository:{milestones:{nodes:[inputs | {title:.}],pageInfo:{hasNextPage:false}}}}}' | out ;;
  *issueOrPullRequest*)
    n=$(printf '%s\n' "$*" | sed -nE 's/.*number=([0-9]+).*/\1/p')
    case " $GH_ISSUES " in
    *" $n "*) printf '{"data":{"repository":{"issueOrPullRequest":{"__typename":"Issue"}}}}\n' | out ;;
    *) printf '{"data":{"repository":{"issueOrPullRequest":null}}}\n' | out ;;
    esac ;;
  esac ;;
"api repos/{owner}/{repo}/milestones") printf '%s\n' "$GH_MILESTONES" ;;
"api repos/{owner}/{repo}/issues/"*)
  n=${2##*/}
  case " $GH_ISSUES " in *" $n "*) printf '%s\n' "$n" ;; *) exit 1 ;; esac
  ;;
"pr list")
  case "$*" in
  *'--json number,url'*) printf '1\thttps://example.com/pull/1\n' ;;
  *) printf '%s\n' "$GH_OPEN_PR" ;;
  esac ;;

"pr view")
  case "$*" in
  *number,url*)
    number=$3
    case $number in *[!0-9]*) number=1 ;; esac
    printf '%s\thttps://example.com/pull/%s\n' "$number" "$number" ;;
  *url*) printf 'https://example.com/pull/%s\n' "$3" ;;
  *) printf '%s\n' "$GH_PR_STATE" ;;
  esac
  ;;
"pr create") printf 'https://example.com/pull/1\n' ;;
"pr edit" | "pr ready") exit 0 ;;
*) exit 0 ;;
esac
EOF
  export GH_POST_LOG="${BATS_TEST_TMPDIR}/gh-post.log"
  : >"$GH_POST_LOG"
  cat >"${BATS_TEST_TMPDIR}/bin/gh-post" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$GH_POST_LOG"
exec gh "$@"
EOF
  chmod +x "${BATS_TEST_TMPDIR}/bin/gh-post"
  chmod +x "${BATS_TEST_TMPDIR}/bin/gh"
  export PATH="${BATS_TEST_TMPDIR}/bin:$PATH"
}

commit_file() {
  printf '%s\n' "$2" >"$1"
  git add -- "$1"
  git commit --quiet -m "$2"
}

# write_draft [milestone] [title] [extra body line]
write_draft() {
  cat >PR_AGENTDESC.md <<EOF
---
base: main
draft: false
labels: [enhancement]
reviewers: []
assignees: []
milestone: ${1:-}
---

# ${2:-feat: add the widget file}

## Summary

Adds the widget file. ${3:-}

## Why

The widget needs a home.

## Verification

Ran \`true\`.
EOF
}

# `! grep` never fails a bats case unless it's the last line, so the
# negative checks go through this.
refute_grep() {
  if grep -q "$@"; then
    return 1
  fi
}

stamp_file() { printf '%s/agent-skills/pr.stamp' "$(git rev-parse --absolute-git-dir)"; }
remote_topic() { git ls-remote origin refs/heads/topic | cut -f1; }

# Keep workflow fixtures independent of the installed size capability.
stub_size_check() {
  export PR_REAL_AGENTREPO
  PR_REAL_AGENTREPO=$(command -v agentrepo)
  export SIZE_LOG="$BATS_TEST_TMPDIR/size.log"
  : >"$SIZE_LOG"
  cat >"$BATS_TEST_TMPDIR/bin/agentrepo" <<'STUB'
#!/usr/bin/env bash
if [ "$1 $2" = 'size check' ]; then
  printf '%s\n' "$*" >>"$SIZE_LOG"
  if [ "${SIZE_MODE:-}" = old ]; then
    echo 'unknown command "check" for "agentrepo size"' >&2
    exit 1
  fi
  echo '801 code SLOC, 0 prose lines'
  if [ "${SIZE_MODE:-}" = over ]; then
    echo 'split by concern, along commit boundaries, or as a stacked series' >&2
    exit 1
  fi
  exit 0
fi
exec "$PR_REAL_AGENTREPO" "$@"
STUB
  chmod +x "$BATS_TEST_TMPDIR/bin/agentrepo"
  export _DEVTOOLS_AGENTREPO_BIN="$BATS_TEST_TMPDIR/bin/agentrepo"
  export _DEVTOOLS_AGENTREPO_CONFIG=true
}
