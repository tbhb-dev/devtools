#!/usr/bin/env bats
# The pr skill's scripts, as generated: preflight, the validator, the
# script-owned review, and the publisher. Each case runs in a scratch
# repository with a bare origin beside it, no global git configuration,
# and a scratch configuration home. The reviewer is a stub set through
# DEVTOOLS_REVIEW_CMD, gh is a stub on PATH that logs every call, and the
# harness session variables are cleared so the confirmation chain starts
# empty.

setup() {
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
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
  : >"$GH_LOG"
  stub_gh

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
case "$1 $2" in
"auth status") exit 0 ;;
"label list") printf '%s\n' "$GH_LABELS" ;;
"api user") printf '%s\n' "$GH_USER" ;;
"api repos/{owner}/{repo}/milestones") printf '%s\n' "$GH_MILESTONES" ;;
"api repos/{owner}/{repo}/issues/"*)
  n=${2##*/}
  case " $GH_ISSUES " in *" $n "*) printf '%s\n' "$n" ;; *) exit 1 ;; esac
  ;;
"pr list") printf '%s\n' "$GH_OPEN_PR" ;;
"pr view")
  case "$*" in
  *url*) printf 'https://example.com/pull/%s\n' "$3" ;;
  *) printf '{"baseRefName":"main","isDraft":false,"labels":[{"name":"bug"}],"reviewRequests":[],"assignees":[],"milestone":null}\n' ;;
  esac
  ;;
"pr create") printf 'https://example.com/pull/1\n' ;;
"pr edit" | "pr ready") exit 0 ;;
*) exit 0 ;;
esac
EOF
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

# The base moves on origin and the local main is left behind: origin has
# a fix the local branch never saw, and topic is rebuilt on top of it.
stale_local_main() {
  git checkout --quiet main
  git rm --quiet docs/gone.md
  git commit --quiet -m 'fix: merged upstream'
  git push --quiet origin main
  git reset --quiet --hard HEAD~1
  git branch --quiet -D topic
  git checkout --quiet -b topic origin/main
  commit_file widget.txt 'feat: add the widget'
}

@test "review stamps the draft, HEAD, the resolved base and its commit, and the merge base" {
  run bash "$SCRIPTS/review.sh"

  [ "$status" -eq 0 ]
  grep -q "^head $(git rev-parse HEAD)$" "$(stamp_file)"
  grep -q '^base origin/main$' "$(stamp_file)"
  grep -q "^base_oid $(git rev-parse origin/main)$" "$(stamp_file)"
  grep -q "^mergebase $(git merge-base origin/main HEAD)$" "$(stamp_file)"
  grep -q '^note none$' "$(stamp_file)"
}

@test "review reads the branch against origin's base, not a stale local one (#146)" {
  stale_local_main
  export DEVTOOLS_REVIEW_CMD="cat >/dev/null; cat \"\$DEVTOOLS_REVIEW_INPUTS/commits.txt\" >'${BATS_TEST_TMPDIR}/seen'; echo 'VERDICT: PASS'"

  run bash "$SCRIPTS/review.sh"

  [ "$status" -eq 0 ]
  grep -q 'feat: add the widget' "${BATS_TEST_TMPDIR}/seen"
  refute_grep 'merged upstream' "${BATS_TEST_TMPDIR}/seen"
}

@test "the validator judges the title's type against origin's base (#146)" {
  stale_local_main
  write_draft '' 'fix: add the widget file'

  run bash "$SCRIPTS/validate-description.sh"

  [ "$status" -eq 1 ]
  [[ "$output" == *"title-type-unused"* ]]
}

@test "the validator judges backticked paths against origin's base (#146)" {
  stale_local_main
  write_draft '' '' 'It replaces `docs/gone.md`.'

  run bash "$SCRIPTS/validate-description.sh"

  [ "$status" -eq 1 ]
  [[ "$output" == *"path-not-found"* ]]
}

@test "the validator reads base: ~ as an empty base, the way the publisher does (#143)" {
  sed 's/^base: main$/base: ~/' PR_AGENTDESC.md > changed
  mv changed PR_AGENTDESC.md

  run bash "$SCRIPTS/validate-description.sh"

  [ "$status" -eq 1 ]
  [[ "$output" == *"frontmatter-empty-base"* ]]
  [[ "$output" != *"base-unknown"* ]]
}

publish_with_milestone() {
  write_draft "$1"
  run bash "$SCRIPTS/validate-description.sh"
  [ "$status" -eq 0 ]
  run bash "$SCRIPTS/review.sh"
  [ "$status" -eq 0 ]
  run bash "$SCRIPTS/create-pr.sh" --confirmed
  [ "$status" -eq 0 ]
  grep -q '^pr create ' "$GH_LOG"
  refute_grep -- '--milestone' "$GH_LOG"
  refute_grep 'milestones' "$GH_LOG"
}

@test "milestone: null publishes with no milestone (#143)" {
  publish_with_milestone null
}

@test "milestone: ~ publishes with no milestone (#143)" {
  publish_with_milestone '~'
}

@test "milestone: \"\" publishes with no milestone (#143)" {
  publish_with_milestone '""'
}

@test "milestone: '' publishes with no milestone (#143)" {
  publish_with_milestone "''"
}

@test "a real milestone title still resolves and publishes (#143 control)" {
  write_draft v1
  bash "$SCRIPTS/review.sh"

  run bash "$SCRIPTS/create-pr.sh" --confirmed

  [ "$status" -eq 0 ]
  grep -q 'milestones' "$GH_LOG"
  grep -q -- '--milestone v1' "$GH_LOG"
}

@test "create-pr publishes a new branch, then checks origin holds the reviewed commit" {
  bash "$SCRIPTS/review.sh"

  run bash "$SCRIPTS/create-pr.sh" --confirmed

  [ "$status" -eq 0 ]
  [[ "$output" == *"the push creates it"* ]]
  [ "$(remote_topic)" = "$(git rev-parse HEAD)" ]
  grep -q '^pr create --base main --head topic --title feat: add the widget file' "$GH_LOG"
  [[ "$output" == *"gh_login is not configured, so the account check is skipped"* ]]
}

@test "create-pr refuses to push a rebased branch without --rewrite (#141)" {
  bash "$SCRIPTS/review.sh"
  bash "$SCRIPTS/create-pr.sh" --confirmed
  before=$(remote_topic)
  : >"$GH_LOG"
  git commit --quiet --amend -m 'feat: add the widget again'
  bash "$SCRIPTS/review.sh"

  run bash "$SCRIPTS/create-pr.sh" --confirmed

  [ "$status" -eq 1 ]
  [[ "$output" == *"not a fast-forward"* ]]
  [[ "$output" == *"--rewrite"* ]]
  [ "$(remote_topic)" = "$before" ]
  refute_grep '^pr \(create\|edit\)' "$GH_LOG"
}

@test "create-pr --rewrite leases on the exact commit origin holds (#141)" {
  bash "$SCRIPTS/review.sh"
  bash "$SCRIPTS/create-pr.sh" --confirmed
  before=$(remote_topic)
  git commit --quiet --amend -m 'feat: add the widget again'
  bash "$SCRIPTS/review.sh"
  export GH_OPEN_PR=7

  run bash "$SCRIPTS/create-pr.sh" --confirmed --rewrite

  [ "$status" -eq 0 ]
  [[ "$output" == *"REWRITE: topic on origin went from $before to $(git rev-parse HEAD)"* ]]
  [ "$(remote_topic)" = "$(git rev-parse HEAD)" ]
  grep -q '^pr edit 7 --base main --title' "$GH_LOG"
  grep -q '^pr edit 7 --remove-label bug' "$GH_LOG"
}

@test "a dry run warns when the real push would not fast-forward (#141)" {
  bash "$SCRIPTS/review.sh"
  bash "$SCRIPTS/create-pr.sh" --confirmed
  before=$(remote_topic)
  git commit --quiet --amend -m 'feat: add the widget again'
  bash "$SCRIPTS/review.sh"
  : >"$GH_LOG"

  run bash "$SCRIPTS/create-pr.sh" --dry-run

  [ "$status" -eq 0 ]
  [[ "$output" == *"WARNING: origin has topic at $before, which HEAD does not contain"* ]]
  [[ "$output" == *"dry run: nothing confirmed, pushed, or published"* ]]
  [ "$(remote_topic)" = "$before" ]
  refute_grep '^pr \(create\|edit\)' "$GH_LOG"
}

@test "create-pr --rewrite refuses when HEAD is behind origin (#141)" {
  commit_file extra.txt 'feat: add more'
  git push --quiet origin HEAD:refs/heads/topic
  git reset --quiet --hard HEAD~1
  bash "$SCRIPTS/review.sh"

  run bash "$SCRIPTS/create-pr.sh" --confirmed --rewrite

  [ "$status" -eq 1 ]
  [[ "$output" == *"HEAD is behind origin"* ]]
}

@test "create-pr --rewrite refuses when origin holds a commit never fetched here (#141)" {
  bash "$SCRIPTS/review.sh"
  bash "$SCRIPTS/create-pr.sh" --confirmed
  other="${BATS_TEST_TMPDIR}/other"
  git clone --quiet --branch topic "$ORIGIN" "$other"
  git -C "$other" -c user.name=Other -c user.email=other@example.com commit --quiet --allow-empty -m 'chore: elsewhere'
  git -C "$other" push --quiet origin topic
  pushed=$(remote_topic)
  git commit --quiet --amend -m 'feat: add the widget again'
  bash "$SCRIPTS/review.sh"

  run bash "$SCRIPTS/create-pr.sh" --confirmed --rewrite

  [ "$status" -eq 1 ]
  [[ "$output" == *"never fetched"* ]]
  [ "$(remote_topic)" = "$pushed" ]
}

@test "create-pr refuses without a review" {
  run bash "$SCRIPTS/create-pr.sh" --confirmed

  [ "$status" -eq 1 ]
  [[ "$output" == *"no independent review has cleared this draft"* ]]
}

@test "create-pr refuses a branch that moved after the review" {
  bash "$SCRIPTS/review.sh"
  commit_file more.txt 'feat: add more'

  run bash "$SCRIPTS/create-pr.sh" --confirmed

  [ "$status" -eq 1 ]
  [[ "$output" == *"Review it again"* ]]
  [ -z "$(remote_topic)" ]
}

@test "create-pr refuses once a fetch moves the base after the review" {
  bash "$SCRIPTS/review.sh"
  other="${BATS_TEST_TMPDIR}/other"
  git clone --quiet "$ORIGIN" "$other"
  git -C "$other" -c user.name=Other -c user.email=other@example.com commit --quiet --allow-empty -m 'chore: base moves'
  git -C "$other" push --quiet origin main
  git fetch --quiet origin

  run bash "$SCRIPTS/create-pr.sh" --confirmed

  [ "$status" -eq 1 ]
  [[ "$output" == *"base_oid changed after the review"* ]]
}

@test "create-pr holds with no confirmation from anyone" {
  bash "$SCRIPTS/review.sh"

  run bash "$SCRIPTS/create-pr.sh"

  [ "$status" -eq 75 ]
  [[ "$output" == *"HELD"* ]]
  [ -z "$(remote_topic)" ]
}

@test "create-pr goes ahead under this session's pre-approval" {
  bash "$SCRIPTS/review.sh"
  mkdir -p .git/preapprovals
  printf 'pr 2026-01-01T00:00:00Z unconfirmed\n' >.git/preapprovals/thread-1

  CODEX_THREAD_ID=thread-1 run bash "$SCRIPTS/create-pr.sh"

  [ "$status" -eq 0 ]
  [[ "$output" == *"confirmation: pre-approval for session thread-1"* ]]
}

@test "create-pr refuses before any write when gh is signed in as another account" {
  bash "$SCRIPTS/review.sh"
  export DEVTOOLS_GH_LOGIN=agent GH_USER=owner

  run bash "$SCRIPTS/create-pr.sh" --confirmed

  [ "$status" -eq 1 ]
  [[ "$output" == *"gh is signed in as owner, not agent"* ]]
  [ -z "$(remote_topic)" ]
  refute_grep '^pr create' "$GH_LOG"
}

@test "create-pr publishes as the configured gh_login" {
  bash "$SCRIPTS/review.sh"
  export DEVTOOLS_GH_LOGIN=agent

  run bash "$SCRIPTS/create-pr.sh" --confirmed

  [ "$status" -eq 0 ]
  [[ "$output" == *"gh identity: agent"* ]]
}

@test "create-pr still resolves every issue the description cites" {
  write_draft '' '' 'Closes #99.'
  bash "$SCRIPTS/review.sh"

  run bash "$SCRIPTS/create-pr.sh" --confirmed

  [ "$status" -eq 1 ]
  [[ "$output" == *"references #99"* ]]
  [ -z "$(remote_topic)" ]
}

@test "create-pr refuses any flag outside its allowlist" {
  run bash "$SCRIPTS/create-pr.sh" --force

  [ "$status" -eq 1 ]
  [[ "$output" == *"usage: create-pr.sh"* ]]
}

@test "preflight reports the confirmation sources, the reviewer, and the gh identity" {
  export DEVTOOLS_GH_LOGIN=agent

  run bash "$SCRIPTS/preflight.sh"

  [ "$status" -eq 0 ]
  [[ "$output" == *"pr:    not granted"* ]]
  [[ "$output" == *"delegation: none configured"* ]]
  [[ "$output" == *"gh identity: create-pr.sh publishes only while gh is signed in as agent"* ]]
  [[ "$output" == *"reviewer: review_cmd:"* ]]
}

@test "PR review reports absent policy and retains credentials floor" {
  export DEVTOOLS_REVIEW_CMD='cat >prompt; echo "VERDICT: PASS"'
  run bash "$SCRIPTS/review.sh"
  [ "$status" -eq 0 ]
  [[ "$output" == *"sha256: none; status: absent"* ]]
  grep -q '^policy_hash none$' .git/agent-skills/pr.stamp
  grep -q '## Credentials floor (non-overridable)' prompt
}
