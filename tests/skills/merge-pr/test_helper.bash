HEAD_OID=1111111111111111111111111111111111111111
BASE_OID=2222222222222222222222222222222222222222
# cspell:words baseword

merge_setup() {
  source "$(dirname "${BASH_SOURCE[0]}")/../../test_helper.bash"
  isolate_git
  ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
  SCRIPTS="${ROOT}/packages/agents/claude/.apm/skills/merge-pr/scripts"
  REPO="${BATS_TEST_TMPDIR}/repo"
  unset CLAUDE_CODE_SESSION_ID CODEX_THREAD_ID CLAUDECODE DEVTOOLS_HARNESS
  unset DEVTOOLS_REVIEW_MODEL DEVTOOLS_REVIEW_EFFORT DEVTOOLS_DELEGATION_CHECK DEVTOOLS_GH_LOGIN
  export XDG_CONFIG_HOME="${BATS_TEST_TMPDIR}/config"
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo "VERDICT: PASS"'
  export GH_FIXTURES="${BATS_TEST_TMPDIR}/gh"
  mkdir -p "$GH_FIXTURES/api" "${BATS_TEST_TMPDIR}/bin"
  export PATH="${BATS_TEST_TMPDIR}/bin:$PATH"
  export MERGE_STUB="$ROOT/tests/skills/merge-pr/gh-merge.bash"
  export CSPELL_COMMIT_MSG="$ROOT/scripts/cspell-commit-msg.sh"
  write_stubs
  write_fixtures

  git init --quiet --initial-branch=main "$REPO"
  git -C "$REPO" config user.name Tester
  git -C "$REPO" config user.email tester@example.com
  cd "$REPO"
  printf 'SQUASH_AGENTMSG\n' >.gitignore
  cat >.cspell.jsonc <<'JSON'
{"dictionaryDefinitions":[{"name":"project","path":"./.cspell-words.txt","addWords":true}],"dictionaries":["project"]}
JSON
  printf 'baseword\n' >.cspell-words.txt
  git add -- .gitignore .cspell.jsonc .cspell-words.txt
  git commit --quiet -m 'test: initial state'
  BASE_OID=$(git rev-parse HEAD)
  git checkout --quiet -b topic
  printf 'a\n' >a.txt
  git add -- a.txt
  git commit --quiet -m 'feat: add the thing'
  HEAD_OID=$(git rev-parse HEAD)
  git clone --quiet --bare . "$GH_FIXTURES/upstream.git"
  # Cloning does not copy the local identity used by the merge stub.
  git -C "$GH_FIXTURES/upstream.git" config user.name Tester
  git -C "$GH_FIXTURES/upstream.git" config user.email tester@example.com
  printf '%s\n' "$GH_FIXTURES/upstream.git" >"$GH_FIXTURES/repository"
  pr ".headRefOid = \"$HEAD_OID\" | .commits[0].oid = \"$HEAD_OID\""
  base_at "$BASE_OID"
  draft
}

write_stubs() {
  # gh: every pull request read returns the fixture whatever fields it
  # names, filtered through jq the way gh's own --jq would.
  cat >"${BATS_TEST_TMPDIR}/bin/gh" <<'STUB'
#!/usr/bin/env bash
F=$GH_FIXTURES
printf '%s\n' "$*" >>"$F/gh.log"
filter=""
json_fields=""
args=()
while [ $# -gt 0 ]; do
  case $1 in
  --jq | -q) filter=$2; shift 2 ;;
  --json) json_fields=$2; shift 2 ;;
  *) args+=("$1"); shift ;;
  esac
done
out() { if [ -n "$filter" ]; then jq -r "$filter"; else cat; fi; }
view_fields() {
  if [ -n "$json_fields" ]; then
    jq --arg fields "$json_fields" '($fields | split(",")) as $fields | with_entries(select(.key as $key | $fields | index($key)))'
  else
    cat
  fi
}
case "${args[0]} ${args[1]:-}" in
"auth status") exit 0 ;;
"pr view")
  if [ -f "$F/live-error" ]; then
    cat "$F/live-error" >&2
    exit "$(cat "$F/live-status")"
  fi
  # $F/unknown holds how many more reads report mergeability as still
  # being computed; "always" never stops.
  n=$(cat "$F/unknown" 2>/dev/null || echo 0)
  if [ "$n" = always ] || [ "$n" -gt 0 ]; then
    [ "$n" = always ] || echo $((n - 1)) >"$F/unknown"
    jq '.mergeable = "UNKNOWN" | .mergeStateStatus = "UNKNOWN"' "$F/pr.json" | view_fields | out
  else
    jq '(.reviews[]?.author) |= del(.__typename)' "$F/pr.json" | view_fields | out
  fi
  ;;
"pr list") if [ -f "$F/children.json" ]; then out <"$F/children.json"; else echo '[]' | out; fi ;;
"pr edit") exit 0 ;;
"pr diff") echo "HTTP 406: diff exceeds 20000 lines" >&2; exit 1 ;;
"repo view")
  if [ "$filter" = .defaultBranchRef.name ]; then echo main; else cat "$F/repository"; fi ;;
"pr merge") source "$MERGE_STUB"; gh_merge ;;

"api rate_limit") echo 1792195200 ;;
"api user") echo 'HTTP 403: API rate limit exceeded' >&2; exit 1 ;;
"api graphql")
  if [ -n "${GH_GRAPHQL_ERROR:-}" ]; then
    echo "$GH_GRAPHQL_ERROR" >&2
    exit "${GH_GRAPHQL_STATUS:-1}"
  fi
  case "${args[*]}" in
  *viewer*) jq '{data:{viewer:.}}' "$F/user.json" | out ;;
  *'author{__typename login}'*)
    jq '[{data:{repository:{pullRequest:{headRefOid:(.reviewHead // .headRefOid),reviews:{nodes:.reviews,pageInfo:{hasNextPage:false,endCursor:null}}}}}}]' "$F/pr.json" | out ;;
  *qualifiedName*)
    if [ -f "$F/base-error" ]; then cat "$F/base-error" >&2; exit 1; fi
    jq '{data:{repository:{ref:{target:{oid:.object.sha}}}}}' "$F/api/repos_{owner}_{repo}_git_ref_heads_main.json" | out ;;
  *issueOrPullRequest*)
    n=$(printf '%s\n' "${args[*]}" | sed -nE 's/.*number=([0-9]+).*/\1/p')
    file="$F/api/repos_{owner}_{repo}_issues_${n}.json"
    if [ -f "$file" ]; then
      jq '{data:{repository:{issueOrPullRequest: (if has("pull_request") then {__typename:"PullRequest"} else {__typename:"Issue",state:(.state|ascii_upcase)} end)}}}' "$file" | out
    else echo '{"data":{"repository":{"issueOrPullRequest":null}}}' | out; fi ;;
  esac ;;

"api "*)
  file=$F/api/$(printf '%s' "${args[1]}" | tr '/' '_').json
  [ -f "$file" ] || { echo "HTTP 404: Not Found" >&2; exit 1; }
  out <"$file"
  ;;
*) echo "gh stub: unhandled: ${args[*]}" >&2; exit 1 ;;
esac
STUB
  cat >"${BATS_TEST_TMPDIR}/bin/mise" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$GH_FIXTURES/mise.log"
case "$1 $2" in
"task info") [ -z "${MISE_NO_TASK:-}" ] ;;
"run lint-squash-msg")
  if [ "${MISE_REAL_SQUASH_LINT:-0}" = 1 ]; then
    exec "$CSPELL_COMMIT_MSG" SQUASH_AGENTMSG
  fi
  exit "${MISE_LINT_STATUS:-0}"
  ;;
*) exit 1 ;;
esac
STUB
  chmod +x "${BATS_TEST_TMPDIR}/bin/gh" "${BATS_TEST_TMPDIR}/bin/mise"
}

write_fixtures() {
  jq -n --arg head "$HEAD_OID" '{
    number: 7, title: "feat: add the thing", state: "OPEN", isDraft: false,
    author: {login: "someone"}, baseRefName: "main", headRefName: "topic",
    headRefOid: $head, isCrossRepository: false, mergeable: "MERGEABLE",
    mergeStateStatus: "CLEAN", reviewDecision: "", url: "https://github.com/o/r/pull/7",
    statusCheckRollup: [
      {__typename: "CheckRun", workflowName: "CI", name: "test", status: "COMPLETED",
       conclusion: "SUCCESS", startedAt: "2026-01-01T00:00:00Z"}
    ],
    commits: [{oid: "3333333333333333333333333333333333333333", messageHeadline: "feat: add the thing",
               messageBody: "Because it was missing.\n\nSigned-off-by: Tester <tester@example.com>",
               authors: [{name: "Tester"}]}],
    body: "## Summary\n\nThe thing.\n",
    files: [{path: "a.txt", additions: 1, deletions: 0}],
    changedFiles: 1, additions: 1, deletions: 0
  }' >"$GH_FIXTURES/pr.json"
  printf 'diff --git a/a.txt b/a.txt\n+a\n' >"$GH_FIXTURES/pr.diff"
  printf '{"login": "agent"}\n' >"$GH_FIXTURES/user.json"
  base_at "$BASE_OID"
}

# pr <jq expression>: change the pull request fixture.
pr() {
  jq "$1" "$GH_FIXTURES/pr.json" >"$GH_FIXTURES/pr.json.new"
  mv "$GH_FIXTURES/pr.json.new" "$GH_FIXTURES/pr.json"
}

base_at() {
  printf '{"object": {"sha": "%s"}}\n' "$1" >"$GH_FIXTURES/api/repos_{owner}_{repo}_git_ref_heads_main.json"
}

draft() {
  printf 'feat: add the thing (#7)\n\nThe thing was missing.\n\nSigned-off-by: Tester <tester@example.com>\n' >SQUASH_AGENTMSG
}

stamp_file() { printf '%s/agent-skills/squash.stamp' "$(git rev-parse --absolute-git-dir)"; }

grant() {
  export CLAUDE_CODE_SESSION_ID=session-1
  mkdir -p "$(git rev-parse --absolute-git-dir)/preapprovals"
  printf '%s 2026-01-01T00:00:00Z touchid\n' "$@" >"$(git rev-parse --absolute-git-dir)/preapprovals/session-1"
}

# A cancelled first run of CI/test, replaced by a later run that passed.
superseded_cancel() {
  pr '.statusCheckRollup = [
    {__typename: "CheckRun", workflowName: "CI", name: "test", status: "COMPLETED",
     conclusion: "CANCELLED", startedAt: "2026-01-01T00:00:00Z"},
    {__typename: "CheckRun", workflowName: "CI", name: "test", status: "COMPLETED",
     conclusion: "SUCCESS", startedAt: "2026-01-01T00:00:05Z"},
    {__typename: "StatusContext", context: "ci/legacy", state: "SUCCESS"}
  ]'
}

lone_cancel() {
  pr '.statusCheckRollup = [
    {__typename: "CheckRun", workflowName: "CI", name: "test", status: "COMPLETED",
     conclusion: "CANCELLED", startedAt: "2026-01-01T00:00:00Z"},
    {__typename: "CheckRun", workflowName: "CI", name: "lint", status: "COMPLETED",
     conclusion: "SUCCESS", startedAt: "2026-01-01T00:00:05Z"}
  ]'
}
