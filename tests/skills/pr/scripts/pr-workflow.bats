#!/usr/bin/env bats
# The pr skill's scripts, as generated: preflight, the validator, the
# script-owned review, and the publisher. Each case runs in a scratch
# repository with a bare origin beside it, no global git configuration,
# and a scratch configuration home. The reviewer is a stub set through
# DEVTOOLS_REVIEW_CMD, gh is a stub on PATH that logs every call, and the
# harness session variables are cleared so the confirmation chain starts
# empty.

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  pr_setup
}

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

@test "create-pr runs the repository pre-push hook before publication" {
  cat >.git/hooks/pre-push <<'HOOK'
#!/usr/bin/env bash
cat >"$BATS_TEST_TMPDIR/pre-push.input"
HOOK
  chmod +x .git/hooks/pre-push
  bash "$SCRIPTS/review.sh"

  run bash "$SCRIPTS/create-pr.sh" --confirmed

  [ "$status" -eq 0 ]
  grep -q ' refs/heads/topic ' "$BATS_TEST_TMPDIR/pre-push.input"
  grep -q '^pr create ' "$GH_LOG"
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
  [[ "$output" == *"reference #99"* ]]
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

@test "identity succeeds without REST even when REST user is rate limited" {
  export DEVTOOLS_GH_LOGIN=agent
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/create-pr.sh" --confirmed --dry-run
  [ "$status" -eq 0 ]
  grep -q 'viewer{login}' "$GH_LOG"
  refute_grep '^api user' "$GH_LOG"
  refute_grep '^auth status' "$GH_LOG"
}

@test "publisher reports a rate limit separately from identity failure" {
  bash "$SCRIPTS/review.sh"
  export GH_VIEWER_ERROR='gh: API rate limit exceeded for user ID 1. (HTTP 403)' GH_ERROR_STATUS=1
  run bash "$SCRIPTS/create-pr.sh" --confirmed
  [ "$status" -eq 76 ]
  [[ "$output" == *"rate limit: GitHub primary GraphQL rate limit, inferred from the command and the error line; the reset time isn't known from the failure"* ]]
  [[ "$output" == *"read the budget with a GraphQL rateLimit"* ]]
  refute_grep 'rate_limit' "$GH_LOG"
  [[ "$output" != *"signed in as"* ]]
  [ -z "$(remote_topic)" ]
}

@test "publisher preserves authentication failures without claiming quota exhaustion" {
  bash "$SCRIPTS/review.sh"
  export GH_VIEWER_ERROR='HTTP 401: Bad credentials' GH_ERROR_STATUS=4
  run bash "$SCRIPTS/create-pr.sh" --confirmed
  [ "$status" -eq 4 ]
  [[ "$output" == *"Bad credentials"* ]]
  [[ "$output" != *"quota exhausted"* ]]
  refute_grep '^api rate_limit' "$GH_LOG"
  [ -z "$(remote_topic)" ]
}


@test "publisher resolves milestones and issue references without REST" {
  write_draft v1 '' 'Closes #12.'
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/create-pr.sh" --confirmed --dry-run
  [ "$status" -eq 0 ]
  grep -q 'milestones(first:100' "$GH_LOG"
  grep -q 'issueOrPullRequest' "$GH_LOG"
  refute_grep '^api repos/' "$GH_LOG"
}

@test "delegated pr checks its script-owned stamp and checker refusal then grant (#137)" {
  printf 'printf "widget\\n"\n' >widget.txt
  git add -- widget.txt
  git commit --quiet --amend -m 'feat: provide a runnable widget example'
  write_draft "" 'feat: provide a runnable widget example' 'Run it with bash widget.txt.'
  sed -e 's/The widget needs a home./Users need a minimal command that prints the widget name./' \
    -e 's/Ran `true`./Ran `bash widget.txt`; it printed the widget name./' \
    PR_AGENTDESC.md >draft.tmp
  mv draft.tmp PR_AGENTDESC.md
  [ "$(bash widget.txt)" = widget ]
  mkdir -p .claude/skills
  cp -R "$ROOT/packages/agents/claude/.apm/skills/pr" \
    "$ROOT/packages/agents/claude/.apm/skills/review-pr-description" .claude/skills/
  SCRIPTS="$PWD/.claude/skills/pr/scripts"
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
  bash "$SCRIPTS/review.sh"
  [ -s "$(stamp_file)" ]

  CHECK_STATUS=1 run bash -c 'source "$1/lib.sh"; dt_confirm pr false && bash "$1/create-pr.sh" --dry-run' _ "$SCRIPTS"
  [ "$status" -eq 1 ]
  [[ "$output" == *REFUSED* ]]
  [ -s "$(stamp_file)" ]

  CHECK_STATUS=0 run bash -c 'source "$1/lib.sh"; dt_confirm pr false && bash "$1/create-pr.sh" --dry-run' _ "$SCRIPTS"
  [ "$status" -eq 0 ]
  [[ "$output" == *"confirmation: delegation_check granted pr in repo"* ]]
  [ "$(cat "$CHECK_ARGS")" = $'check repo pr\ncheck repo pr' ]
  [ -z "$(remote_topic)" ]
  refute_grep '^pr create' "$GH_LOG"
}

@test "updating a PR rejects fields printed before jq fails" {
  bash "$SCRIPTS/review.sh"
  export GH_OPEN_PR=1 REAL_JQ FAIL_FILTER
  REAL_JQ=$(command -v jq)
  cat >"${BATS_TEST_TMPDIR}/bin/jq" <<'STUB'
#!/usr/bin/env bash
"$REAL_JQ" "$@" || exit $?
[ "$*" != "-r $FAIL_FILTER" ] || exit 9
STUB
  chmod +x "${BATS_TEST_TMPDIR}/bin/jq"
  for FAIL_FILTER in '.labels[].name' '.reviewRequests[].login // empty' '.assignees[].login' '.milestone.title // empty'; do
    run bash "$SCRIPTS/create-pr.sh" --confirmed
    [ "$status" -eq 9 ]
  done
}

@test "description failure leaves the test log untouched (#198)" {
  export FAIL_GATE=lint-pr-description
  test_log=.git/agent-skills/gates/9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08.log
  mkdir -p .git/agent-skills/gates
  printf 'earlier test output\n' >"$test_log"
  run bash "$SCRIPTS/create-pr.sh" --dry-run
  [ "$status" -ne 0 ]
  [ "$(cat "$GATE_LOG")" = lint-pr-description ]
  [ "$(cat "$test_log")" = 'earlier test output' ]
}

@test "check-description runs only description checks without a review (#198)" {
  run bash "$SCRIPTS/create-pr.sh" --check-description
  [ "$status" -eq 0 ]
  [ "$(cat "$GATE_LOG")" = lint-pr-description ]
  [ ! -f .git/agent-skills/pr.stamp ]
  [ -z "$(remote_topic)" ]
}

@test "dry run then publication reuses gates but runs scans and review (#161)" {
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo review >>"$GATE_LOG"; echo "VERDICT: PASS"'
  bash "$SCRIPTS/review.sh"
  : >"$GATE_LOG"
  bash "$SCRIPTS/create-pr.sh" --dry-run
  run bash "$SCRIPTS/create-pr.sh" --confirmed
  [ "$status" -eq 0 ]
  [[ "$output" == *"mise run test passed at"*"(reused)"* ]]
  [ "$(cat "$GATE_LOG")" = $'lint-pr-description\ndevtools:lint-leaks\ndevtools:gitleaks\nlint\nreview\ntest\ndevtools:lint-leaks\ndevtools:gitleaks\nlint\nreview' ]
}

@test "check-description rejects an unknown label before other gates (#198)" {
  export GH_LABELS=bug
  run bash "$SCRIPTS/create-pr.sh" --check-description
  [ "$status" -ne 0 ]
  [[ "$output" == *"no label named"* ]]
  [ "$(cat "$GATE_LOG")" = lint-pr-description ]
}

@test "a scripted review failure exits before test (#198)" {
  bash "$SCRIPTS/review.sh"
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo "VERDICT: CHANGES REQUIRED"; echo finding'
  run bash "$SCRIPTS/create-pr.sh" --dry-run
  [ "$status" -ne 0 ]
  refute_grep '^test$' "$GATE_LOG"
}

@test "publisher rerun-gates overrides cached passes (#161)" {
  bash "$SCRIPTS/review.sh"
  bash "$SCRIPTS/create-pr.sh" --dry-run
  run bash "$SCRIPTS/create-pr.sh" --dry-run --rerun-gates
  [ "$status" -eq 0 ]
  [[ "$output" != *"(reused)"* ]]
  [ "$(grep -c '^test$' "$GATE_LOG")" -eq 2 ]
}

@test "preflight does not require the optional description writer (#225)" {
  run bash "$SCRIPTS/preflight.sh"
  [ "$status" -eq 0 ]
  if [[ "$output" == *"write-pr-description skill deployed:"* ]]; then return 1; fi
  [[ "$output" == *"review-pr-description skill deployed:"* ]]
}

@test "PR creation and content edits go through gh-post" {
  run bash "$SCRIPTS/review.sh"
  [ "$status" -eq 0 ]
  run bash "$SCRIPTS/create-pr.sh" --confirmed
  [ "$status" -eq 0 ]
  grep -q '^pr create .*--body-file ' "$GH_POST_LOG"
  export GH_OPEN_PR=1
  run bash "$SCRIPTS/create-pr.sh" --confirmed
  [ "$status" -eq 0 ]
  grep -q '^pr edit 1 .*--title .*--body-file ' "$GH_POST_LOG"
}

@test "section findings name the template and keep their line and rule (#550)" {
  printf '## Summary\n\n## Why\n\n## Verification\n\n## Risks\n' >.github/pull_request_template.md
  write_draft
  printf '\n## Risks\n\n## Extra\n\nnotes\n' >>PR_AGENTDESC.md

  run bash "$SCRIPTS/validate-description.sh"

  [ "$status" -eq 1 ]
  [[ "$output" == *"PR_AGENTDESC.md:"*" [error] section-empty  \"## Risks\" has no content under it; template: .github/pull_request_template.md"* ]]

  printf '## Summary\n\n## Why\n\n## Verification\n\n## Rollout\n' >.github/pull_request_template.md
  write_draft '' 'fix: add the widget file'
  run bash "$SCRIPTS/validate-description.sh"
  [[ "$output" == *" [error] section-missing  the draft has no \"## Rollout\" section; the template puts it after \"## Verification\"; template: .github/pull_request_template.md"* ]]
  [[ "$output" == *"title-type-unused"* ]]
  refute_grep 'title-type-unused.*template:' <<<"$output"

  printf '## Why\n\n## Summary\n\n## Verification\n' >.github/pull_request_template.md
  write_draft
  run bash "$SCRIPTS/validate-description.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *" [error] section-order  the template's sections run in this order: Why, Summary, Verification; template: .github/pull_request_template.md"* ]]
}

@test "section findings name the template that supplied the requirements (#550)" {
  printf '## Alpha\n\ncontent\n' >.github/env.md
  printf '## Beta\n\ncontent\n' >.github/repo.md

  PR_TEMPLATE=.github/env.md run bash "$SCRIPTS/validate-description.sh"
  [[ "$output" == *"the draft has no \"## Alpha\" section; the template opens with it; template: .github/env.md"* ]]

  mkdir -p .agents
  echo 'pr_template = .github/repo.md' >.agents/agent-skills.conf
  PR_TEMPLATE=.github/env.md run bash "$SCRIPTS/validate-description.sh"
  [[ "$output" == *"the draft has no \"## Beta\" section; the template opens with it; template: .github/repo.md"* ]]
  [[ "$output" != *"Alpha"* ]]
}

@test "the template-level errors and a clean run are unchanged (#550)" {
  run bash "$SCRIPTS/validate-description.sh"
  [ "$status" -eq 0 ]
  [ -z "$output" ]

  printf 'no headings\n' >.github/pull_request_template.md
  run bash "$SCRIPTS/validate-description.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *"template-has-no-sections  .github/pull_request_template.md declares no level 2 headings"* ]]

  rm .github/pull_request_template.md
  run bash "$SCRIPTS/validate-description.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *"no pull request template at .github/pull_request_template.md"* ]]
}

# A mise that runs the real scanner for the leak task, the way the task
# file maps the scope variables, and records the scope each task saw.
real_leak_scanner() {
  cat >"${BATS_TEST_TMPDIR}/bin/mise" <<STUB
#!/usr/bin/env bash
case "\$1" in
--version) echo mise-test ;;
ls) echo '{}' ;;
run)
  printf '%s %s %s\n' "\$2" "\${DEVTOOLS_LEAK_MODE-}" "\${DEVTOOLS_LEAK_BASE-}" >>"\$GATE_LOG"
  if [ "\$2" = devtools:lint-leaks ]; then
    shift 2
    [ "\${DEVTOOLS_LEAK_MODE-}" != publish ] || set -- --publish
    bash "$ROOT/.devtools/scripts/check-leaks.sh" "\$@"
  fi
  ;;
esac
STUB
  cat >.git/hooks/pre-push <<HOOK
#!/usr/bin/env bash
DEVTOOLS_LEAK_MODE=\${DEVTOOLS_LEAK_MODE-} bash "$ROOT/.devtools/scripts/check-leaks.sh" --pre-push-hook "\$@"
HOOK
  chmod +x .git/hooks/pre-push
  export O11Y_PRIVATE_STRINGS=kestrel99
}

# The base already holds a list value in a file the branch edits, in a file
# it leaves alone, and in a path name.
leaky_base() {
  git checkout --quiet main
  printf 'kestrel99\nclean\n' >edited.txt
  printf 'kestrel99\n' >untouched.txt
  : >kestrel99-name.txt
  git add -- edited.txt untouched.txt kestrel99-name.txt
  git commit --quiet -m 'chore: base holds list values'
  git push --quiet origin main
  git checkout --quiet topic
  git rebase --quiet main
  git fetch --quiet origin
}

@test "AC1 publication passes all three scans when the branch adds no list value (#651)" {
  leaky_base
  real_leak_scanner
  printf 'kestrel99\nclean\nmore\n' >edited.txt
  git commit --quiet -am 'feat: edit a file holding a base value'
  bash "$SCRIPTS/review.sh"

  run bash "$SCRIPTS/create-pr.sh" --confirmed

  [ "$status" -eq 0 ]
  [ "$(remote_topic)" = "$(git rev-parse HEAD)" ]
  merge_base=$(git merge-base HEAD origin/main)
  grep -q "^devtools:lint-leaks publish $merge_base$" "$GATE_LOG"
  grep -q "^lint publish $merge_base$" "$GATE_LOG"
}

@test "AC2 publication stops on an added list value, before the push (#651)" {
  leaky_base
  real_leak_scanner
  printf 'kestrel99\nclean\nkestrel99\n' >edited.txt
  git commit --quiet -am 'feat: add a value'
  bash "$SCRIPTS/review.sh"

  run bash "$SCRIPTS/create-pr.sh" --confirmed

  [ "$status" -ne 0 ]
  [[ "$output" == *"edited.txt:3: private literal"* ]]
  [ -z "$(remote_topic)" ]
}

@test "the pre-push hook alone stops an added list value (#651)" {
  leaky_base
  real_leak_scanner
  printf 'kestrel99\nclean\nkestrel99\n' >edited.txt
  git commit --quiet -am 'feat: add a value'
  export DEVTOOLS_LEAK_MODE=publish DEVTOOLS_LEAK_BASE
  DEVTOOLS_LEAK_BASE=$(git merge-base HEAD origin/main)

  run git push origin topic

  [ "$status" -ne 0 ]
  [ -z "$(remote_topic)" ]
}

@test "AC3 a missing, shallow or unrelated base exits 2 before any push (#651)" {
  real_leak_scanner
  main_oid=$(git rev-parse origin/main)
  git update-ref -d refs/remotes/origin/main
  run bash "$SCRIPTS/create-pr.sh" --confirmed
  [ "$status" -eq 2 ]
  [[ "$output" == *"origin/main is missing"* ]]
  [ -z "$(remote_topic)" ]

  git update-ref refs/remotes/origin/main "$main_oid"
  git rev-parse HEAD~1 >.git/shallow
  run bash "$SCRIPTS/create-pr.sh" --confirmed
  [ "$status" -eq 2 ]
  [[ "$output" == *"the repository is shallow"* ]]
  [ -z "$(remote_topic)" ]
  rm .git/shallow

  git update-ref refs/remotes/origin/main "$(git commit-tree -m orphan "$(git rev-parse 'HEAD^{tree}')")"
  run bash "$SCRIPTS/create-pr.sh" --confirmed
  [ "$status" -eq 2 ]
  [[ "$output" == *"share no history"* ]]
  [ -z "$(remote_topic)" ]
  refute_grep '^devtools:lint-leaks' "$GATE_LOG"
}

@test "AC7 publication runs lint uncached under the scope (#651)" {
  real_leak_scanner
  bash "$SCRIPTS/review.sh"
  bash "$SCRIPTS/create-pr.sh" --dry-run
  : >"$GATE_LOG"
  run bash "$SCRIPTS/create-pr.sh" --dry-run
  [ "$status" -eq 0 ]
  [[ "$output" != *"mise run lint passed at"* ]]
  grep -q '^lint publish ' "$GATE_LOG"
}

@test "AC3 publication without either base ref exits 2 before any push (#651)" {
  git update-ref -d refs/remotes/origin/main
  git branch -D main
  run bash "$SCRIPTS/create-pr.sh"
  [ "$status" -eq 2 ]
  [[ "$output" == *"origin/main is missing; the leak scan is incomplete. Nothing was pushed."* ]]
  [ -z "$(remote_topic)" ]
  refute_grep '^devtools:lint-leaks' "$GATE_LOG"
  refute_grep '^pr create ' "$GH_LOG"
}
