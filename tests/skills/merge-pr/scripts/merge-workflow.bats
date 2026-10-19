#!/usr/bin/env bats
# The merge-pr skill's scripts, as generated: preflight, the squash
# message briefing, the script-owned review, and the merge wrapper. Each
# case runs in a scratch repository with no global git configuration and
# a scratch configuration home. gh and mise are stubs on PATH that read
# fixtures and log what they were asked; the reviewer is a stub set
# through DEVTOOLS_REVIEW_CMD; the harness session variables are cleared
# so the confirmation chain starts empty.

HEAD_OID=1111111111111111111111111111111111111111
BASE_OID=2222222222222222222222222222222222222222

setup_file() {
  local source_root
  source_root=$(git -C "$BATS_TEST_DIRNAME" rev-parse --show-toplevel)
  (cd "$source_root" && go build -o "$BATS_FILE_TMPDIR/commit-lint" ./cmd/commit-lint)
}

setup() {
  export PATH="$BATS_FILE_TMPDIR:$PATH"
  source "${BATS_TEST_DIRNAME}/../../../test_helper.bash"
  isolate_git
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
  BASE_OID=$(git rev-parse HEAD)
  git checkout --quiet -b topic
  printf 'a\n' >a.txt
  git add -- a.txt
  git commit --quiet -m 'feat: add the thing'
  HEAD_OID=$(git rev-parse HEAD)
  git clone --quiet --bare . "$GH_FIXTURES/upstream.git"
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
  if [ -f "$F/live-error" ]; then
    cat "$F/live-error" >&2
    exit "$(cat "$F/live-status")"
  fi
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
"pr diff") echo "HTTP 406: diff exceeds 20000 lines" >&2; exit 1 ;;
"repo view") cat "$F/repository" ;;
"pr merge")
  printf '%s\n' "${args[*]}" >"$F/merged"
  case ${MERGE_RESULT:-merged} in
  merged | lost | missing-commit | wrong-head)
    jq '.state = "MERGED" | .mergeCommit = {oid:"4444444444444444444444444444444444444444"}' "$F/pr.json" >"$F/next.json"
    mv "$F/next.json" "$F/pr.json"
    case ${MERGE_RESULT:-merged} in
    lost) echo 'response lost' >&2; exit 1 ;;
    missing-commit) jq '.mergeCommit = null' "$F/pr.json" >"$F/next.json"; mv "$F/next.json" "$F/pr.json" ;;
    wrong-head) jq '.headRefOid = "different"' "$F/pr.json" >"$F/next.json"; mv "$F/next.json" "$F/pr.json" ;;
    esac ;;
  queued) ;;
  refused) echo 'merge refused' >&2; exit 1 ;;
  unavailable)
    echo 'read-back unavailable' >"$F/live-error"
    echo 1 >"$F/live-status" ;;
  esac ;;

"api rate_limit") echo 1792195200 ;;
"api user") echo 'HTTP 403: API rate limit exceeded' >&2; exit 1 ;;
"api graphql")
  if [ -n "${GH_GRAPHQL_ERROR:-}" ]; then
    echo "$GH_GRAPHQL_ERROR" >&2
    exit "${GH_GRAPHQL_STATUS:-1}"
  fi
  case "${args[*]}" in
  *viewer*) jq '{data:{viewer:.}}' "$F/user.json" | out ;;
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
  [ "$(grep -c '^pr view ' "$GH_FIXTURES/gh.log")" -eq 1 ]
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

  [ "$status" -eq 75 ]
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

@test "required reviews wait before preflight or reviewer work (#170)" {
  for decision in REVIEW_REQUIRED CHANGES_REQUESTED; do
    pr ".reviewDecision = \"$decision\" | .mergeStateStatus = \"BLOCKED\""
    for script in preflight review; do
      DEVTOOLS_REVIEW_CMD='touch "$GH_FIXTURES/reviewer-started"; echo "VERDICT: PASS"' \
        run bash "$SCRIPTS/$script.sh" 7
      [ "$status" -eq 75 ]
      [[ "$output" == *"WAITING_REVIEW: #7"* ]]
      [ ! -e "$GH_FIXTURES/reviewer-started" ]
      [ ! -e "$(stamp_file)" ]
    done
  done
}

@test "required review reason takes priority over BLOCKED at merge (#170)" {
  pr '.reviewDecision = "APPROVED"'
  bash "$SCRIPTS/review.sh" 7
  pr '.reviewDecision = "REVIEW_REQUIRED" | .mergeStateStatus = "BLOCKED"'
  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7
  [ "$status" -eq 75 ]
  [[ "$output" == *"WAITING_REVIEW: #7"* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}

@test "approved required review merges with the reviewed head (#170)" {
  pr '.reviewDecision = "APPROVED"'
  bash "$SCRIPTS/review.sh" 7
  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7
  [ "$status" -eq 0 ]
  grep -q -- "--match-head-commit $HEAD_OID" "$GH_FIXTURES/merged"
}

@test "review does not stamp when required approval changes during review (#170)" {
  DEVTOOLS_REVIEW_CMD='cat >/dev/null; jq ".reviewDecision = \"REVIEW_REQUIRED\"" "$GH_FIXTURES/pr.json" >"$GH_FIXTURES/x" && mv "$GH_FIXTURES/x" "$GH_FIXTURES/pr.json"; echo "VERDICT: PASS"' \
    run bash "$SCRIPTS/review.sh" 7
  [ "$status" -eq 75 ]
  [[ "$output" == *"WAITING_REVIEW: #7"* ]]
  [ ! -e "$(stamp_file)" ]
}

# Capture exactly what reaches the independent reviewer, before it can
# read a live branch or a working tree instead of the pinned snapshot.
capture_diff() {
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; cp "$DEVTOOLS_REVIEW_INPUTS/change.diff" "$GH_FIXTURES/reviewed.diff"; cp "$DEVTOOLS_REVIEW_INPUTS/files.txt" "$GH_FIXTURES/reviewed.files"; echo "VERDICT: PASS"'
}

@test "review receives a complete local diff above GitHub's line limit (#152)" {
  git clone --quiet "$GH_FIXTURES/upstream.git" "$GH_FIXTURES/producer"
  producer=$GH_FIXTURES/producer
  git -C "$producer" config user.name Tester
  git -C "$producer" config user.email tester@example.com
  seq 1 21001 >"$producer/large.txt"
  printf '\000\001\002' >"$producer/binary.dat"
  git -C "$producer" add -- large.txt binary.dat
  git -C "$producer" commit --quiet -m 'feat: large payload'
  HEAD_OID=$(git -C "$producer" rev-parse HEAD)
  git -C "$producer" push --quiet origin HEAD:refs/pull/7/head
  pr ".headRefOid = \"$HEAD_OID\""
  git -C "$producer" diff --no-ext-diff --no-textconv --no-color --binary --full-index "$BASE_OID...$HEAD_OID" -- >"$GH_FIXTURES/expected.diff"
  # The pin is absent locally and the checked-out files are unrelated.
  ! git cat-file -e "$HEAD_OID" 2>/dev/null
  printf 'uncommitted\n' >a.txt
  capture_diff

  run bash "$SCRIPTS/review.sh" 7

  [ "$status" -eq 0 ]
  [ "$(wc -l <"$GH_FIXTURES/reviewed.diff")" -gt 20000 ]
  cmp "$GH_FIXTURES/expected.diff" "$GH_FIXTURES/reviewed.diff"
  grep -q 'GIT binary patch' "$GH_FIXTURES/reviewed.diff"
  grep -q 'large.txt$' "$GH_FIXTURES/reviewed.files"
  grep -q 'binary.dat$' "$GH_FIXTURES/reviewed.files"
  ! grep -q '^pr diff' "$GH_FIXTURES/gh.log"
  [ "$(cat a.txt)" = uncommitted ]
}

@test "review fetches fork heads from the base repository without origin (#152)" {
  pr '.isCrossRepository = true | .headRefName = "fork-only"'
  git --git-dir="$GH_FIXTURES/upstream.git" update-ref refs/pull/7/head "$HEAD_OID"
  git --git-dir="$GH_FIXTURES/upstream.git" update-ref -d refs/heads/topic
  capture_diff
  run bash "$SCRIPTS/review.sh" 7
  [ "$status" -eq 0 ]
  git diff --no-ext-diff --no-textconv --no-color --binary --full-index "$BASE_OID...$HEAD_OID" -- >"$GH_FIXTURES/expected.diff"
  cmp "$GH_FIXTURES/expected.diff" "$GH_FIXTURES/reviewed.diff"
}

@test "review keeps its pinned diff and refuses a base moved during review (#152)" {
  git diff --no-ext-diff --no-textconv --no-color --binary --full-index "$BASE_OID...$HEAD_OID" -- >"$GH_FIXTURES/expected.diff"
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; cp "$DEVTOOLS_REVIEW_INPUTS/change.diff" "$GH_FIXTURES/reviewed.diff"; printf "{\"object\":{\"sha\":\"5555555555555555555555555555555555555555\"}}\n" >"$GH_FIXTURES/api/repos_{owner}_{repo}_git_ref_heads_main.json"; echo "VERDICT: PASS"'
  run bash "$SCRIPTS/review.sh" 7
  [ "$status" -eq 5 ]
  cmp "$GH_FIXTURES/expected.diff" "$GH_FIXTURES/reviewed.diff"
  [ ! -e "$(stamp_file)" ]
}

@test "review refuses missing pinned objects before invoking reviewer (#152)" {
  pr '.headRefOid = "4444444444444444444444444444444444444444"'
  capture_diff
  run bash "$SCRIPTS/review.sh" 7
  [ "$status" -eq 1 ]
  [[ "$output" == *"Could not fetch the pinned base and head"* ]]
  [ ! -e "$GH_FIXTURES/reviewed.diff" ]
  [ ! -e "$(stamp_file)" ]
}

@test "review uses the merge base when the target has unrelated commits (#152)" {
  git checkout --quiet main
  printf 'base-only\n' >base-only.txt
  git add -- base-only.txt
  git commit --quiet -m 'test: advance base'
  BASE_OID=$(git rev-parse HEAD)
  git push --quiet "$GH_FIXTURES/upstream.git" main
  base_at "$BASE_OID"
  capture_diff
  run bash "$SCRIPTS/review.sh" 7
  [ "$status" -eq 0 ]
  git diff --no-ext-diff --no-textconv --no-color --binary --full-index "$BASE_OID...$HEAD_OID" -- >"$GH_FIXTURES/expected.diff"
  cmp "$GH_FIXTURES/expected.diff" "$GH_FIXTURES/reviewed.diff"
  ! grep -q base-only "$GH_FIXTURES/reviewed.diff"
}

@test "review completes shallow history before constructing the pinned diff (#152)" {
  git clone --quiet --depth=1 --branch topic "file://$GH_FIXTURES/upstream.git" "$GH_FIXTURES/shallow"
  cd "$GH_FIXTURES/shallow"
  [ "$(git rev-parse --is-shallow-repository)" = true ]
  draft
  capture_diff
  run bash "$SCRIPTS/review.sh" 7
  [ "$status" -eq 0 ]
  [ "$(git rev-parse --is-shallow-repository)" = false ]
  git diff --no-ext-diff --no-textconv --no-color --binary --full-index "$BASE_OID...$HEAD_OID" -- >"$GH_FIXTURES/expected.diff"
  cmp "$GH_FIXTURES/expected.diff" "$GH_FIXTURES/reviewed.diff"
}

@test "merge review reports absent policy and retains credentials floor" {
  export DEVTOOLS_REVIEW_CMD='cat >prompt; echo "VERDICT: PASS"'
  run bash "$SCRIPTS/review.sh" 7
  [ "$status" -eq 0 ]
  [[ "$output" == *"sha256: none; status: absent"* ]]
  grep -q '^policy_hash none$' .git/agent-skills/squash.stamp
  grep -q '## Credentials floor (non-overridable)' prompt
}


@test "merge preflight reports quota reset with a distinct status" {
  export GH_GRAPHQL_ERROR='HTTP 403: API rate limit exceeded'
  run bash "$SCRIPTS/preflight.sh" 7
  [ "$status" -eq 76 ]
  [[ "$output" == *"reset time: 1792195200"* ]]
  [[ "$output" != *"NOT the configured"* ]]
}

@test "merge preflight preserves authentication failure status" {
  export GH_GRAPHQL_ERROR='HTTP 401: Bad credentials' GH_GRAPHQL_STATUS=4
  run bash "$SCRIPTS/preflight.sh" 7
  [ "$status" -eq 4 ]
  [[ "$output" == *"Bad credentials"* ]]
  [[ "$output" != *"quota exhausted"* ]]
}

@test "merge readers use GraphQL rather than REST user and refs" {
  draft
  run bash "$SCRIPTS/preflight.sh" 7
  [ "$status" -eq 0 ]
  run bash "$SCRIPTS/review.sh" 7
  [ "$status" -eq 0 ]
  if grep -E '^api (user|repos/)' "$GH_FIXTURES/gh.log"; then return 1; fi
  grep -q 'qualifiedName' "$GH_FIXTURES/gh.log"
}


@test "quota failure during final review validation keeps status 76 and no stamp" {
  draft
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo "HTTP 403: API rate limit exceeded" >"$GH_FIXTURES/live-error"; echo 1 >"$GH_FIXTURES/live-status"; echo "VERDICT: PASS"'
  run bash "$SCRIPTS/review.sh" 7
  [ "$status" -eq 76 ]
  [[ "$output" == *"reset time: 1792195200"* ]]
  [ ! -e "$(stamp_file)" ]
}

@test "auth failure during final review validation keeps its status and no stamp" {
  draft
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo "HTTP 401: Bad credentials" >"$GH_FIXTURES/live-error"; echo 4 >"$GH_FIXTURES/live-status"; echo "VERDICT: PASS"'
  run bash "$SCRIPTS/review.sh" 7
  [ "$status" -eq 4 ]
  [[ "$output" == *"Bad credentials"* ]]
  [[ "$output" != *"quota exhausted"* ]]
  [ ! -e "$(stamp_file)" ]
}


@test "base ref quota failure during final review validation keeps status 76" {
  draft
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo "HTTP 403: API rate limit exceeded" >"$GH_FIXTURES/base-error"; echo "VERDICT: PASS"'
  run bash "$SCRIPTS/review.sh" 7
  [ "$status" -eq 76 ]
  [[ "$output" == *"reset time: 1792195200"* ]]
  [ ! -e "$(stamp_file)" ]
}

@test "delegated merge checks its script-owned stamp and checker refusal then grant (#137)" {
  mkdir -p .claude/skills
  cp -R "$ROOT/packages/agents/claude/.apm/skills/merge-pr" \
    "$ROOT/packages/agents/claude/.apm/skills/review-squash-message" .claude/skills/
  SCRIPTS="$PWD/.claude/skills/merge-pr/scripts"
  # Opt-in CLI smoke test; ordinary runs keep the deterministic reviewer.
  if [ "${RUN_HEADLESS_REVIEWS:-0}" = 1 ]; then
    unset DEVTOOLS_REVIEW_CMD
    export DEVTOOLS_HARNESS=claude
  fi
  # No skill hooks, operator flag, or session pre-approval participates.
  [ -z "${CLAUDE_CODE_SESSION_ID:-}${CODEX_THREAD_ID:-}" ]
  export DEVTOOLS_DELEGATION_CHECK="${BATS_TEST_TMPDIR}/checker"
  export CHECK_ARGS="${BATS_TEST_TMPDIR}/checker.args"
  cat >"$DEVTOOLS_DELEGATION_CHECK" <<'CHECKER'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$CHECK_ARGS"
exit "${CHECK_STATUS:-1}"
CHECKER
  chmod +x "$DEVTOOLS_DELEGATION_CHECK"
  bash "$SCRIPTS/review.sh" 7
  [ -s "$(stamp_file)" ]

  CHECK_STATUS=1 run bash "$SCRIPTS/squash-merge.sh" 7
  [ "$status" -eq 1 ]
  [[ "$output" == *REFUSED* ]]
  [ -s "$(stamp_file)" ]

  CHECK_STATUS=0 run bash "$SCRIPTS/squash-merge.sh" 7
  [ "$status" -eq 0 ]
  [[ "$output" == *"confirmation: delegation_check granted merge in repo"* ]]
  [ "$(cat "$CHECK_ARGS")" = $'check repo merge\ncheck repo merge' ]
  grep -q -- "--match-head-commit $HEAD_OID" "$GH_FIXTURES/merged"
}

@test "mergeability and draft checks reject fields printed before jq fails" {
  bash "$SCRIPTS/review.sh" 7
  export REAL_JQ FAIL_FIELD
  REAL_JQ=$(command -v jq)
  cat >"${BATS_TEST_TMPDIR}/bin/jq" <<'STUB'
#!/usr/bin/env bash
"$REAL_JQ" "$@" || exit $?
case " $* " in *" --arg f $FAIL_FIELD "*) exit 9 ;; esac
STUB
  chmod +x "${BATS_TEST_TMPDIR}/bin/jq"
  for FAIL_FIELD in mergeable mergeStateStatus isDraft; do
    run bash "$SCRIPTS/squash-merge.sh" --confirmed 7
    [ "$status" -eq 9 ]
    [ ! -f "$GH_FIXTURES/merged" ]
  done
}

# Every result must come from one merge attempt and one read-back.
merge_result() {
  export MERGE_RESULT=$1
  bash "$SCRIPTS/review.sh" 7
  printf 'draft\n' >PR_AGENTDESC.md
  cp "$(stamp_file)" "$(dirname "$(stamp_file)")/pr.stamp"
  git remote add origin "$GH_FIXTURES/upstream.git"
  git --git-dir="$GH_FIXTURES/upstream.git" symbolic-ref HEAD refs/heads/main
  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7
  [ "$(grep -c '^pr merge ' "$GH_FIXTURES/gh.log")" -eq 1 ]
  [ "$(sed -n '/^pr merge /,$p' "$GH_FIXTURES/gh.log" | grep -c '^pr view ')" -eq 1 ]
}

assert_preserved() {
  [ -s SQUASH_AGENTMSG ]
  [ -s PR_AGENTDESC.md ]
  [ -s "$(stamp_file)" ]
  [ -s "$(dirname "$(stamp_file)")/pr.stamp" ]
  git --git-dir="$GH_FIXTURES/upstream.git" show-ref --verify refs/heads/topic
}

assert_cleaned() {
  [ ! -e SQUASH_AGENTMSG ]
  [ ! -e PR_AGENTDESC.md ]
  [ ! -e "$(stamp_file)" ]
  [ ! -e "$(dirname "$(stamp_file)")/pr.stamp" ]
  ! git --git-dir="$GH_FIXTURES/upstream.git" show-ref --verify refs/heads/topic
  [[ "$output" == *"MERGED"* ]]
  [[ "$output" == *"4444444444444444444444444444444444444444"* ]]
}

@test "merge result: confirmed merge cleans up and records the commit" {
  merge_result merged
  [ "$status" -eq 0 ]
  assert_cleaned
  grep -q 4444444444444444444444444444444444444444 "$(dirname "$(stamp_file)")/merge.result.json"
}

@test "merge result: queued merge preserves drafts stamps and branch" {
  merge_result queued
  [ "$status" -eq 75 ]
  [[ "$output" == *"PENDING"* ]]
  [[ "$output" == *"gh pr view 7"* ]]
  assert_preserved
}

@test "merge result: lost response after server merge still succeeds" {
  merge_result lost
  [ "$status" -eq 0 ]
  assert_cleaned
}

@test "merge result: refused merge reads back before reporting failure" {
  merge_result refused
  [ "$status" -eq 1 ]
  [[ "$output" == *"FAILED"* ]]
  assert_preserved
}

@test "merge result: unavailable read-back leaves the outcome unknown" {
  merge_result unavailable
  [ "$status" -eq 1 ]
  [[ "$output" == *"UNKNOWN"* ]]
  assert_preserved
}

@test "merge result: merged without a commit leaves the outcome unknown" {
  merge_result missing-commit
  [ "$status" -eq 1 ]
  [[ "$output" == *"UNKNOWN"* ]]
  assert_preserved
}

@test "merge result: merged at another head leaves the outcome unknown" {
  merge_result wrong-head
  [ "$status" -eq 1 ]
  [[ "$output" == *"UNKNOWN"* ]]
  assert_preserved
}

@test "squash assembly drops denied agents while retaining people and bots" {
  printf '[identities]\nagents = [{email = "agent@example.com"}]\n' >.commit-lint.toml
  pr '.commits[0].messageBody = "Reason.\n\nSigned-off-by: Agent <agent@example.com>\nSigned-off-by: Tester <tester@example.com>\nSigned-off-by: Renovate <renovate[bot]@users.noreply.github.com>"'
  run bash "$SCRIPTS/squash-message.sh" 7
  [ "$status" -eq 0 ]
  grep -Fq 'Dropped sign-off: Signed-off-by: Agent <agent@example.com>' <<<"$output"
  run grep -q 'Signed-off-by: Agent' SQUASH_AGENTMSG
  [ "$status" -eq 1 ]
  grep -Fx 'Signed-off-by: Tester <tester@example.com>' SQUASH_AGENTMSG
  grep -Fx 'Signed-off-by: Renovate <renovate[bot]@users.noreply.github.com>' SQUASH_AGENTMSG
}

@test "squash assembly fails on invalid identity configuration" {
  printf '[identities]\nagents = [{name = "Agent"}]\n' >.commit-lint.toml
  run bash "$SCRIPTS/squash-message.sh" 7
  [ "$status" -eq 2 ]
  grep -Fq "identity email pattern must not be empty" <<<"$output"
}

@test "squash assembly drops mixed-case agent sign-offs recognized by Git" {
  printf '[identities]\nagents = [{email = "agent@example.com"}]\n' >.commit-lint.toml
  pr '.commits[0].messageBody = "Reason.\n\nsIgNeD-oFf-By: Agent <agent@example.com>\nSigned-off-by: Tester <tester@example.com>"'
  run bash "$SCRIPTS/squash-message.sh" 7
  [ "$status" -eq 0 ]
  grep -Fq 'Dropped sign-off: sIgNeD-oFf-By: Agent <agent@example.com>' <<<"$output"
  run grep -qi 'signed-off-by: Agent' SQUASH_AGENTMSG
  [ "$status" -eq 1 ]
  grep -Fx 'Signed-off-by: Tester <tester@example.com>' SQUASH_AGENTMSG
}

@test "merge refuses a squash message without attribution for agent commits" {
  printf '[identities]\nagents = [{email = "tester@example.com"}]\n' >.commit-lint.toml
  bash "$SCRIPTS/review.sh" 7

  run bash "$SCRIPTS/squash-merge.sh" --confirmed 7

  [ "$status" -eq 1 ]
  [[ "$output" == *"configured agent author or committer requires a well-formed Assisted-by trailer"* ]]
  [[ "$output" == *"squash message lacks attribution required by the commits it collapses"* ]]
  [ ! -e "$GH_FIXTURES/merged" ]
}
