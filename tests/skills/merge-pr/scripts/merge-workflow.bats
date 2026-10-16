#!/usr/bin/env bats
# The merge-pr skill's scripts, as generated: preflight, the squash
# message briefing, the script-owned review, and the merge wrapper. Each
# case runs in a scratch repository with no global git configuration and
# a scratch configuration home. gh and mise are stubs on PATH that read
# fixtures and log what they were asked; the reviewer is a stub set
# through DEVTOOLS_REVIEW_CMD; the harness session variables are cleared
# so the confirmation chain starts empty.

readonly HEAD_OID=1111111111111111111111111111111111111111
readonly BASE_OID=2222222222222222222222222222222222222222

setup() {
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
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
  write_stubs
  write_fixtures

  git init --quiet --initial-branch=main "$REPO"
  git -C "$REPO" config user.name Tester
  git -C "$REPO" config user.email tester@example.com
  cd "$REPO"
  printf 'SQUASH_AGENTMSG\n' >.gitignore
  git add -- .gitignore
  git commit --quiet -m 'test: initial state'
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
args=()
while [ $# -gt 0 ]; do
  case $1 in
  --jq | -q) filter=$2; shift 2 ;;
  --json) shift 2 ;;
  *) args+=("$1"); shift ;;
  esac
done
out() { if [ -n "$filter" ]; then jq -r "$filter"; else cat; fi; }
case "${args[0]} ${args[1]:-}" in
"auth status") exit 0 ;;
"pr view")
  # $F/unknown holds how many more reads report mergeability as still
  # being computed; "always" never stops.
  n=$(cat "$F/unknown" 2>/dev/null || echo 0)
  if [ "$n" = always ] || [ "$n" -gt 0 ]; then
    [ "$n" = always ] || echo $((n - 1)) >"$F/unknown"
    jq '.mergeable = "UNKNOWN" | .mergeStateStatus = "UNKNOWN"' "$F/pr.json" | out
  else
    out <"$F/pr.json"
  fi
  ;;
"pr list") echo '[]' | out ;;
"pr diff") cat "$F/pr.diff" ;;
"pr merge") printf '%s\n' "${args[*]}" >"$F/merged" ;;
"api user") out <"$F/user.json" ;;
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
"run lint-squash-msg") exit "${MISE_LINT_STATUS:-0}" ;;
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

@test "squash-message writes Closes only for an open issue under Related (#134)" {
  pr '.body = "## Summary\n\nThe thing.\n\n## Related\n\n- #10 the bug\n- #11 the base of this stack\n- #12 an old report\n- tbhb/other#99\n- #7 itself\n"'
  printf '{"number": 10, "state": "open"}\n' >"$GH_FIXTURES/api/repos_{owner}_{repo}_issues_10.json"
  printf '{"number": 11, "state": "open", "pull_request": {"url": "x"}}\n' >"$GH_FIXTURES/api/repos_{owner}_{repo}_issues_11.json"
  printf '{"number": 12, "state": "closed"}\n' >"$GH_FIXTURES/api/repos_{owner}_{repo}_issues_12.json"

  run bash "$SCRIPTS/squash-message.sh" 7

  [ "$status" -eq 0 ]
  grep -qx 'Closes #10' SQUASH_AGENTMSG
  ! grep -q 'Closes #11' SQUASH_AGENTMSG
  ! grep -q 'Closes #12' SQUASH_AGENTMSG
  ! grep -q '#99' SQUASH_AGENTMSG
  ! grep -q 'Not closed' SQUASH_AGENTMSG
  [[ "$output" == *"# Not closed: #11 is a pull request"* ]]
  [[ "$output" == *"# Not closed: #12 is a closed issue"* ]]
}

@test "preflight leaves a superseded cancelled run out of the checks (#145)" {
  superseded_cancel

  run bash "$SCRIPTS/preflight.sh" 7

  [ "$status" -eq 0 ]
  [[ "$output" == *"summary: all green."* ]]
  [[ "$output" == *"1 earlier run(s) left out"* ]]
  [[ "$output" == *"head oid: $HEAD_OID"* ]]
}

@test "preflight still fails a cancelled run nothing replaced (#145)" {
  lone_cancel

  run bash "$SCRIPTS/preflight.sh" 7

  [ "$status" -eq 0 ]
  [[ "$output" == *"summary: FAILING"* ]]
  [[ "$output" == *"CANCELLED"*"CI / test"* ]]
}

@test "preflight reports a merge grant and leaves a stale draft alone" {
  grant merge
  printf 'fix: old (#3)\n' >SQUASH_AGENTMSG

  run bash "$SCRIPTS/preflight.sh" 7

  [ "$status" -eq 0 ]
  [[ "$output" == *"merge: GRANTED"* ]]
  [[ "$output" == *"SQUASH_AGENTMSG: STALE"* ]]
  [ -s SQUASH_AGENTMSG ]
}

@test "review stamps the draft with the pull request, head, base, and base commit" {
  run bash "$SCRIPTS/review.sh" 7

  [ "$status" -eq 0 ]
  grep -qx 'pr 7' "$(stamp_file)"
  grep -qx "head $HEAD_OID" "$(stamp_file)"
  grep -qx 'base main' "$(stamp_file)"
  grep -qx "base_oid $BASE_OID" "$(stamp_file)"
  grep -qx 'note none' "$(stamp_file)"
}

@test "review stamps nothing when the reviewer returns findings" {
  DEVTOOLS_REVIEW_CMD='cat >/dev/null; printf "VERDICT: CHANGES REQUIRED\n\n1. [coverage] gap\n"' \
    run bash "$SCRIPTS/review.sh" 7

  [ "$status" -eq 2 ]
  [[ "$output" == *"1. [coverage] gap"* ]]
  [ ! -e "$(stamp_file)" ]
}

@test "review refuses to stamp when the head moves while it runs" {
  DEVTOOLS_REVIEW_CMD='cat >/dev/null; jq ".headRefOid = \"4444444444444444444444444444444444444444\"" "$GH_FIXTURES/pr.json" >"$GH_FIXTURES/x" && mv "$GH_FIXTURES/x" "$GH_FIXTURES/pr.json"; echo "VERDICT: PASS"' \
    run bash "$SCRIPTS/review.sh" 7

  [ "$status" -eq 5 ]
  [ ! -e "$(stamp_file)" ]
}

@test "squash-merge refuses without a review" {
  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 1 ]
  [[ "$output" == *"no independent review has cleared this message"* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}

@test "squash-merge refuses a head pushed after the review" {
  bash "$SCRIPTS/review.sh" 7
  pr ".headRefOid = \"4444444444444444444444444444444444444444\""

  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 1 ]
  [[ "$output" == *"The head changed after the review"* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}

@test "squash-merge refuses a base that moved after the review" {
  bash "$SCRIPTS/review.sh" 7
  base_at 5555555555555555555555555555555555555555

  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 1 ]
  [[ "$output" == *"The base_oid changed after the review"* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}

@test "squash-merge holds with no confirmation from anyone" {
  bash "$SCRIPTS/review.sh" 7

  run bash "$SCRIPTS/squash-merge.sh" 7

  [ "$status" -eq 75 ]
  [[ "$output" == *"HELD"* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}

@test "squash-merge holds under a bare pre-approval, which never covers merge" {
  bash "$SCRIPTS/review.sh" 7
  grant commit pr

  run bash "$SCRIPTS/squash-merge.sh" 7

  [ "$status" -eq 75 ]
  [ ! -e "$GH_FIXTURES/merged" ]
}

@test "squash-merge merges under a merge pre-approval, pinned to the reviewed head" {
  bash "$SCRIPTS/review.sh" 7
  grant merge
  printf 'draft\n' >PR_AGENTDESC.md

  run bash "$SCRIPTS/squash-merge.sh" 7

  [ "$status" -eq 0 ]
  [[ "$output" == *"confirmation: pre-approval"* ]]
  [[ "$output" == *"gh_login is not configured; skipping the account check"* ]]
  grep -q -- "--match-head-commit $HEAD_OID" "$GH_FIXTURES/merged"
  grep -q -- "--subject feat: add the thing (#7)" "$GH_FIXTURES/merged"
  grep -q -- "--body-file" "$GH_FIXTURES/merged"
  [ ! -e SQUASH_AGENTMSG ]
  [ ! -e PR_AGENTDESC.md ]
  [ ! -e "$(stamp_file)" ]
}

@test "squash-merge merges over a superseded cancelled run (#145)" {
  superseded_cancel
  bash "$SCRIPTS/review.sh" 7

  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 0 ]
  [ -e "$GH_FIXTURES/merged" ]
}

@test "squash-merge refuses a cancelled run nothing replaced (#145)" {
  lone_cancel
  bash "$SCRIPTS/review.sh" 7

  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 1 ]
  [[ "$output" == *"failing check, judged by its latest run"* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}

@test "squash-merge refuses when gh is signed in as another account" {
  bash "$SCRIPTS/review.sh" 7
  export DEVTOOLS_GH_LOGIN=owner

  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 1 ]
  [[ "$output" == *"gh is signed in as agent, not the configured owner"* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}

@test "squash-merge refuses a merge state other than CLEAN" {
  bash "$SCRIPTS/review.sh" 7
  pr '.mergeStateStatus = "BEHIND"'

  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 1 ]
  [[ "$output" == *"not ready to merge (BEHIND)"* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}

@test "squash-merge refuses a review decision asking for changes" {
  bash "$SCRIPTS/review.sh" 7
  pr '.reviewDecision = "CHANGES_REQUESTED"'

  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 1 ]
  [[ "$output" == *"review decision is CHANGES_REQUESTED"* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}

@test "squash-merge refuses when the lint task is absent rather than skipping it" {
  bash "$SCRIPTS/review.sh" 7

  MISE_NO_TASK=1 run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 1 ]
  [[ "$output" == *"lint-squash-msg is absent"* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}

@test "squash-merge waits out an UNKNOWN mergeability and then merges" {
  bash "$SCRIPTS/review.sh" 7
  echo 2 >"$GH_FIXTURES/unknown"

  MERGE_MERGEABLE_POLL=0 run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 0 ]
  [[ "$output" == *"still computing mergeability"* ]]
  [ "$(cat "$GH_FIXTURES/unknown")" = 0 ]
  [ -e "$GH_FIXTURES/merged" ]
}

@test "squash-merge refuses once an UNKNOWN mergeability outlasts the wait" {
  bash "$SCRIPTS/review.sh" 7
  echo always >"$GH_FIXTURES/unknown"

  MERGE_MERGEABLE_POLL=0 MERGE_MERGEABLE_WAIT=3 run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 1 ]
  [[ "$output" == *"still reports its mergeability as UNKNOWN after 3s"* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}

@test "squash-merge refuses a conflicting pull request" {
  bash "$SCRIPTS/review.sh" 7
  pr '.mergeable = "CONFLICTING" | .mergeStateStatus = "DIRTY"'

  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 1 ]
  [[ "$output" == *"not mergeable (CONFLICTING)"* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}

@test "squash-merge leaves a fork's branch alone" {
  pr '.isCrossRepository = true'
  bash "$SCRIPTS/review.sh" 7

  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 0 ]
  [[ "$output" == *"left fork branch topic untouched"* ]]
}

@test "squash-merge accepts a remote branch auto-deleted during cleanup" {
  bash "$SCRIPTS/review.sh" 7
  export REAL_GIT
  REAL_GIT=$(command -v git)
  cat >"${BATS_TEST_TMPDIR}/bin/git" <<'STUB'
#!/usr/bin/env bash
case "$1" in
ls-remote) echo 'head refs/heads/topic'; exit 0 ;;
push) echo 'error: unable to delete topic: remote ref does not exist' >&2; exit 1 ;;
*) exec "$REAL_GIT" "$@" ;;
esac
STUB
  chmod +x "${BATS_TEST_TMPDIR}/bin/git"

  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 0 ]
  [[ "$output" == *"origin/topic was already deleted"* ]]
  [[ "$output" != *"could not delete"* ]]
}
