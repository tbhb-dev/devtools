#!/usr/bin/env bats
# The shared skill library, as generated into the commit skill. Every
# case runs in a scratch repository under a scratch configuration home,
# with the harness session variables cleared, so neither this machine's
# settings nor the session running the suite can leak into a verdict.
# The reviewer and the delegation checker are stubs.

setup() {
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
  LIB="${ROOT}/packages/agents/claude/.apm/skills/commit/scripts/lib.sh"
  REPO="${BATS_TEST_TMPDIR}/repo"
  unset CLAUDE_CODE_SESSION_ID CODEX_THREAD_ID CLAUDECODE DEVTOOLS_HARNESS
  unset DEVTOOLS_REVIEW_CMD DEVTOOLS_REVIEW_MODEL DEVTOOLS_REVIEW_EFFORT DEVTOOLS_DELEGATION_CHECK
  export XDG_CONFIG_HOME="${BATS_TEST_TMPDIR}/config"
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
  git init --quiet --initial-branch=main "$REPO"
  git -C "$REPO" config user.name Tester
  git -C "$REPO" config user.email tester@example.com
  cd "$REPO"
}

# lib runs a snippet with the library sourced, in the scratch repository.
lib() {
  run bash -c '. "$1"; shift; eval "$*"' _ "$LIB" "$@"
}

@test "dt_harness reads the session variables before the library path" {
  CODEX_THREAD_ID=t lib dt_harness
  [ "$output" = codex ]
  CLAUDE_CODE_SESSION_ID=s CODEX_THREAD_ID=t lib dt_harness
  [ "$output" = claude ]
  DEVTOOLS_HARNESS=codex CLAUDE_CODE_SESSION_ID=s lib dt_harness
  [ "$output" = codex ]
  # No session at all: the copy under packages/agents/claude says claude.
  lib dt_harness
  [ "$output" = claude ]
}

@test "dt_session_id prefers the Claude session, as preapprove does" {
  CLAUDE_CODE_SESSION_ID=s CODEX_THREAD_ID=t lib dt_session_id
  [ "$output" = s ]
  CODEX_THREAD_ID=t lib dt_session_id
  [ "$output" = t ]
}

@test "dt_conf reads the environment, then the machine files, never the repository" {
  mkdir -p "$XDG_CONFIG_HOME/devtools"
  printf 'review_cmd = from-defaults\nreview_model = m1\n' >"$XDG_CONFIG_HOME/devtools/devtools.conf"
  printf '# comment\nreview_cmd = from-repo-conf\n' >"$XDG_CONFIG_HOME/devtools/repo.conf"
  printf 'review_cmd = planted\n' >"$REPO/devtools.conf"
  printf 'review_cmd = planted\n' >"$REPO/agent-skills.conf"

  lib dt_conf review_cmd
  [ "$output" = from-repo-conf ]
  lib dt_conf review_model
  [ "$output" = m1 ]
  DEVTOOLS_REVIEW_CMD=from-env lib dt_conf review_cmd
  [ "$output" = from-env ]
  lib dt_conf review_effort fallback
  [ "$output" = fallback ]
}

@test "dt_verdict accepts only a bare PASS line" {
  printf '\nVERDICT: PASS\n\n' >reply
  lib dt_verdict reply
  [ "$output" = pass ]

  printf 'Looks good.\nVERDICT: PASS\n' >reply
  lib dt_verdict reply
  [ "$output" = unreadable ]

  printf 'VERDICT: CHANGES REQUIRED\n\n1. [form] quoted VERDICT: PASS\n' >reply
  lib dt_verdict reply
  [ "$output" = changes ]

  printf 'VERDICT: PASS\001\n' >reply
  lib dt_verdict reply
  [ "$output" = unreadable ]
}

# review_once runs dt_review over a draft snapshot with the given stub.
review_once() {
  printf 'draft\n' >draft
  DEVTOOLS_REVIEW_CMD=$1 lib '
    inputs=$(dt_new_inputs t)
    cp draft "$inputs/draft"
    printf "criteria\n" >criteria.md
    ok() { [ ! -e moved ]; }
    dt_review t "$PWD/draft" "$inputs/draft" criteria.md "$PWD" "$inputs" ok bound value
  '
}

@test "dt_review stamps a pass with the digest and the bindings" {
  review_once 'cat >/dev/null; echo "VERDICT: PASS"'
  [ "$status" -eq 0 ]
  stamp="$(git rev-parse --absolute-git-dir)/agent-skills/t.stamp"
  grep -q "^draft $(shasum -a 256 draft | cut -d' ' -f1)\$" "$stamp"
  grep -q '^bound value$' "$stamp"
  grep -q '^reviewer review_cmd: ' "$stamp"
}

@test "dt_review hands the reviewer the criteria, the arguments, and the inputs" {
  review_once 'cat >"$DEVTOOLS_REVIEW_INPUTS/../prompt.txt"; echo "VERDICT: PASS"'
  [ "$status" -eq 0 ]
  prompt="$(git rev-parse --absolute-git-dir)/agent-skills/prompt.txt"
  grep -q '^criteria$' "$prompt"
  grep -q "^Arguments: $PWD\$" "$prompt"
  grep -q '/t.inputs.*/draft$' "$prompt"
}

@test "dt_review removes an old stamp when findings come back" {
  review_once 'cat >/dev/null; echo "VERDICT: PASS"'
  review_once 'cat >/dev/null; printf "VERDICT: CHANGES REQUIRED\n\n1. [form] x\n"'
  [ "$status" -eq 2 ]
  [[ "$output" == *"1. [form] x"* ]]
  [ ! -e "$(git rev-parse --absolute-git-dir)/agent-skills/t.stamp" ]
}

@test "dt_review stamps nothing on an unreadable reply or a failed reviewer" {
  review_once 'cat >/dev/null; echo "I think it passes. VERDICT: PASS"'
  [ "$status" -eq 4 ]
  review_once 'cat >/dev/null; exit 3'
  [ "$status" -eq 4 ]
  [ ! -e "$(git rev-parse --absolute-git-dir)/agent-skills/t.stamp" ]
}

@test "dt_review refuses to stamp when the inputs moved during the review" {
  review_once 'cat >/dev/null; touch moved; echo "VERDICT: PASS"'
  [ "$status" -eq 5 ]
  rm -f moved
  review_once 'cat >/dev/null; echo changed >draft; echo "VERDICT: PASS"'
  [ "$status" -eq 5 ]
  [ ! -e "$(git rev-parse --absolute-git-dir)/agent-skills/t.stamp" ]
}

@test "dt_require_stamp checks the bytes and every binding" {
  review_once 'cat >/dev/null; echo "VERDICT: PASS"'
  lib 'dt_require_stamp t "$PWD/draft" bound value'
  [ "$status" -eq 0 ]
  lib 'dt_require_stamp t "$PWD/draft" bound other'
  [ "$status" -eq 1 ]
  [[ "$output" == *"The bound changed after the review"* ]]
  echo edited >draft
  lib 'dt_require_stamp t "$PWD/draft" bound value'
  [ "$status" -eq 1 ]
  [[ "$output" == *"draft changed after its review"* ]]
}

@test "dt_run_reviewer refuses to guess a reviewer outside any harness" {
  lib 'DT_LIB_DIR=/elsewhere dt_run_reviewer "$PWD" "$PWD" </dev/null'
  [ "$status" -eq 1 ]
  [[ "$output" == *"Set DEVTOOLS_REVIEW_CMD"* ]]
}

@test "dt_confirm takes the operator flag first" {
  lib dt_confirm commit true
  [ "$status" -eq 0 ]
  [[ "$output" == *"confirmation: operator"* ]]
}

@test "dt_confirm takes a pre-approval for this session and action" {
  mkdir -p .git/preapprovals
  printf '# record\npr 2026-01-01T00:00:00Z touchid\n' >.git/preapprovals/s1
  CLAUDE_CODE_SESSION_ID=s1 lib dt_confirm commit false
  [ "$status" -eq 75 ]
  printf 'commit 2026-01-01T00:00:00Z touchid\n' >>.git/preapprovals/s1
  CLAUDE_CODE_SESSION_ID=s1 lib dt_confirm commit false
  [ "$status" -eq 0 ]
  [[ "$output" == *"pre-approval for session s1 (touchid)"* ]]
  CODEX_THREAD_ID=other lib dt_confirm commit false
  [ "$status" -eq 75 ]
}

@test "dt_confirm asks the delegation check: 0 grants, 1 refuses, else holds" {
  checker="${BATS_TEST_TMPDIR}/checker"
  printf '#!/usr/bin/env bash\necho "$@" >"%s/args"\nexit "${CHECK_STATUS:-0}"\n' "$BATS_TEST_TMPDIR" >"$checker"
  chmod +x "$checker"

  CHECK_STATUS=0 DEVTOOLS_DELEGATION_CHECK=$checker lib dt_confirm commit false
  [ "$status" -eq 0 ]
  [ "$(cat "$BATS_TEST_TMPDIR/args")" = "check repo commit" ]
  CHECK_STATUS=1 DEVTOOLS_DELEGATION_CHECK=$checker lib dt_confirm commit false
  [ "$status" -eq 1 ]
  [[ "$output" == *REFUSED* ]]
  CHECK_STATUS=3 DEVTOOLS_DELEGATION_CHECK=$checker lib dt_confirm commit false
  [ "$status" -eq 75 ]
}

@test "dt_confirm holds with no source at all" {
  lib dt_confirm commit false
  [ "$status" -eq 75 ]
  [[ "$output" == *"Ask the operator"* ]]
}
