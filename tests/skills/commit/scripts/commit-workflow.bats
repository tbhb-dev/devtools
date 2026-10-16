#!/usr/bin/env bats
# The commit skill's scripts, as generated: preflight, the script-owned
# review, and the commit wrapper. Each case runs in a scratch repository
# with no global git configuration and a scratch configuration home. The
# reviewer is a stub set through DEVTOOLS_REVIEW_CMD, and the harness
# session variables are cleared so the confirmation chain starts empty.

setup() {
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
  SCRIPTS="${ROOT}/packages/agents/claude/.apm/skills/commit/scripts"
  REPO="${BATS_TEST_TMPDIR}/repo"
  unset CLAUDE_CODE_SESSION_ID CODEX_THREAD_ID CLAUDECODE DEVTOOLS_HARNESS
  unset DEVTOOLS_REVIEW_MODEL DEVTOOLS_REVIEW_EFFORT DEVTOOLS_DELEGATION_CHECK
  export XDG_CONFIG_HOME="${BATS_TEST_TMPDIR}/config"
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
  export COMMIT_PREFLIGHT_FETCH=0
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo "VERDICT: PASS"'
  git init --quiet --initial-branch=main "$REPO"
  git -C "$REPO" config user.name Tester
  git -C "$REPO" config user.email tester@example.com
  cd "$REPO"
  printf 'test: bind the review\n\nSigned-off-by: Tester <tester@example.com>\n' >COMMIT_AGENTMSG
}

initial_commit() {
  printf 'before\n' >tracked.txt
  git add -- tracked.txt
  git commit --quiet -m 'test: initial state'
}

stage() {
  printf '%s\n' "$2" >"$1"
  git add -- "$1"
}

stamp_file() { printf '%s/agent-skills/commit.stamp' "$(git rev-parse --absolute-git-dir)"; }

@test "preflight reports a root commit on an unborn HEAD with no origin (#138)" {
  stage a.txt a
  git remote add origin "${BATS_TEST_TMPDIR}/missing.git"

  run bash "$SCRIPTS/preflight.sh"

  [ "$status" -eq 0 ]
  [[ "$output" == *"branch:   main"* ]]
  [[ "$output" == *"recommended base: none (root commit)"* ]]
  [[ "$output" == *"commit guard: armed at unborn"* ]]
  [[ "$output" == *"A	a.txt"* ]]
  [ "$(cat .git/commit-workflow.head)" = unborn ]
}

@test "preflight recommends the local default when origin has no copy of it" {
  initial_commit
  git checkout --quiet -b topic

  run bash "$SCRIPTS/preflight.sh"

  [ "$status" -eq 0 ]
  [[ "$output" == *"recommended base: main"* ]]
  [[ "$output" == *"origin/main does not exist"* ]]
}

@test "review and commit make a repository's first commit (#138)" {
  stage a.txt a

  run bash "$SCRIPTS/review.sh"
  [ "$status" -eq 0 ]
  grep -q '^head unborn$' "$(stamp_file)"
  grep -q '^note none$' "$(stamp_file)"

  run bash "$SCRIPTS/commit.sh" --confirmed
  [ "$status" -eq 0 ]
  [ "$(git log -1 --format=%s)" = 'test: bind the review' ]
  [ "$(git rev-list --count HEAD)" = 1 ]
  [ ! -e "$(stamp_file)" ]
}

@test "commit refuses without a review" {
  initial_commit
  stage tracked.txt reviewed

  run bash "$SCRIPTS/commit.sh" --confirmed

  [ "$status" -eq 1 ]
  [[ "$output" == *"No independent review has cleared this draft"* ]]
  [ "$(git log -1 --format=%s)" = 'test: initial state' ]
}

@test "commit refuses a staged tree changed after the review" {
  initial_commit
  stage tracked.txt reviewed
  bash "$SCRIPTS/review.sh"
  stage tracked.txt changed-after-review

  run bash "$SCRIPTS/commit.sh" --confirmed

  [ "$status" -eq 1 ]
  [[ "$output" == *"The tree changed after the review"* ]]
  [ "$(git log -1 --format=%s)" = 'test: initial state' ]
}

@test "commit refuses a draft edited after the review" {
  initial_commit
  stage tracked.txt reviewed
  bash "$SCRIPTS/review.sh"
  printf 'test: something else\n' >COMMIT_AGENTMSG

  run bash "$SCRIPTS/commit.sh" --confirmed

  [ "$status" -eq 1 ]
  [[ "$output" == *"COMMIT_AGENTMSG changed after its review"* ]]
}

@test "review stamps nothing when the reviewer returns findings" {
  initial_commit
  stage tracked.txt reviewed

  DEVTOOLS_REVIEW_CMD='cat >/dev/null; printf "VERDICT: CHANGES REQUIRED\n\n1. [form] subject\n"' \
    run bash "$SCRIPTS/review.sh"

  [ "$status" -eq 2 ]
  [[ "$output" == *"1. [form] subject"* ]]
  [ ! -e "$(stamp_file)" ]
}

@test "review refuses to stamp when the index moves while it runs" {
  initial_commit
  stage tracked.txt reviewed

  DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo moved >tracked.txt; git add -- tracked.txt; echo "VERDICT: PASS"' \
    run bash "$SCRIPTS/review.sh"

  [ "$status" -eq 5 ]
  [ ! -e "$(stamp_file)" ]
}

@test "commit holds with no confirmation from anyone" {
  initial_commit
  stage tracked.txt reviewed
  bash "$SCRIPTS/review.sh"

  run bash "$SCRIPTS/commit.sh"

  [ "$status" -eq 75 ]
  [[ "$output" == *"HELD"* ]]
  [ "$(git log -1 --format=%s)" = 'test: initial state' ]
  [ -e "$(stamp_file)" ]
}

@test "commit goes ahead under this session's pre-approval" {
  initial_commit
  stage tracked.txt reviewed
  bash "$SCRIPTS/review.sh"
  mkdir -p .git/preapprovals
  printf 'commit 2026-01-01T00:00:00Z unconfirmed\n' >.git/preapprovals/thread-1

  CODEX_THREAD_ID=thread-1 run bash "$SCRIPTS/commit.sh"

  [ "$status" -eq 0 ]
  [[ "$output" == *"confirmation: pre-approval for session thread-1 (unconfirmed)"* ]]
  [ "$(git show HEAD:tracked.txt)" = reviewed ]
}

@test "commit follows the delegation check's refusal" {
  initial_commit
  stage tracked.txt reviewed
  bash "$SCRIPTS/review.sh"
  printf '#!/usr/bin/env bash\nexit 1\n' >"${BATS_TEST_TMPDIR}/refuse"
  chmod +x "${BATS_TEST_TMPDIR}/refuse"

  DEVTOOLS_DELEGATION_CHECK="${BATS_TEST_TMPDIR}/refuse" run bash "$SCRIPTS/commit.sh"

  [ "$status" -eq 1 ]
  [[ "$output" == *"REFUSED"* ]]
  [ "$(git log -1 --format=%s)" = 'test: initial state' ]
}

@test "commit refuses any flag outside its allowlist" {
  initial_commit
  stage tracked.txt reviewed
  bash "$SCRIPTS/review.sh"

  run bash "$SCRIPTS/commit.sh" --confirmed --no-verify

  [ "$status" -eq 1 ]
  [[ "$output" == *"Received: --no-verify"* ]]
}

@test "commit keeps a body line opening with a number sign" {
  initial_commit
  stage tracked.txt reviewed
  printf 'test: keep the hash line\n\n# not a comment\n' >COMMIT_AGENTMSG
  bash "$SCRIPTS/review.sh"

  run bash "$SCRIPTS/commit.sh" --confirmed

  [ "$status" -eq 0 ]
  git log -1 --format=%B | grep -q '^# not a comment$'
}

@test "an amend reviews and commits against the parent of HEAD" {
  initial_commit
  stage tracked.txt first
  bash "$SCRIPTS/review.sh"
  bash "$SCRIPTS/commit.sh" --confirmed
  printf 'test: bind the review\n\nSigned-off-by: Tester <tester@example.com>\n' >COMMIT_AGENTMSG
  stage other.txt second

  run bash "$SCRIPTS/commit.sh" --amend --confirmed
  [ "$status" -eq 1 ]

  run bash "$SCRIPTS/review.sh" --amend
  [ "$status" -eq 0 ]
  grep -q '^mode amend$' "$(stamp_file)"
  run bash "$SCRIPTS/commit.sh" --amend --confirmed
  [ "$status" -eq 0 ]
  [ "$(git rev-list --count HEAD)" = 2 ]
  [ "$(git show HEAD:other.txt)" = second ]
}

@test "commit verifies a merge commit against its first parent (#142)" {
  initial_commit
  git checkout --quiet -b side
  stage side.txt side
  git commit --quiet -m 'test: side change'
  git checkout --quiet main
  stage main.txt main
  git commit --quiet -m 'test: main change'
  git merge --quiet --no-ff --no-commit side
  printf 'test: merge side into main\n' >COMMIT_AGENTMSG
  bash "$SCRIPTS/review.sh"

  run bash "$SCRIPTS/commit.sh" --confirmed

  [ "$status" -eq 0 ]
  [[ "$output" != *"short of what was staged"* ]]
  [ "$(git rev-list --parents -n 1 HEAD | wc -w)" -eq 3 ]
}

@test "commit fails, without undoing it, when a hook changes the committed tree" {
  initial_commit
  stage tracked.txt reviewed
  bash "$SCRIPTS/review.sh"
  mkdir -p .git/hooks
  printf '#!/usr/bin/env bash\necho hooked >extra.txt\ngit add -- extra.txt\n' >.git/hooks/pre-commit
  chmod +x .git/hooks/pre-commit

  run bash "$SCRIPTS/commit.sh" --confirmed

  [ "$status" -eq 1 ]
  [[ "$output" == *"differs from the staged tree the reviewer cleared"* ]]
  [ "$(git log -1 --format=%s)" = 'test: bind the review' ]
}

@test "review copies the operator note into the prompt and stamps its digest" {
  initial_commit
  stage tracked.txt reviewed
  printf 'The operator settled the grouping because the deployment is shared.\n' >note.txt

  DEVTOOLS_REVIEW_CMD='cat >"$DEVTOOLS_REVIEW_INPUTS/../prompt.txt"; echo "VERDICT: PASS"' \
    run bash "$SCRIPTS/review.sh" --note note.txt --amend

  [ "$status" -eq 0 ]
  state="$(git rev-parse --absolute-git-dir)/agent-skills"
  grep -q '^## Settled by the operator$' "$state/prompt.txt"
  grep -qF "$(cat note.txt)" "$state/prompt.txt"
  grep -q '^mode amend$' "$(stamp_file)"
  grep -q "^note $(shasum -a 256 note.txt | cut -d' ' -f1)\$" "$(stamp_file)"
  cmp note.txt "$state"/commit.inputs.*/operator-note.txt
}

@test "review refuses a missing operator note file" {
  initial_commit
  stage tracked.txt reviewed

  run bash "$SCRIPTS/review.sh" --note missing.txt

  [ "$status" -eq 1 ]
  [[ "$output" == *"Operator note is not a file: missing.txt"* ]]
  [ ! -e "$(stamp_file)" ]
}
