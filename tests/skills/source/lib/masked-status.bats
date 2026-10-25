#!/usr/bin/env bats
# Producers deliberately print valid output before failing. Invoke functions
# in a conditional too: errexit cannot rescue a masked status there.
setup() {
  source "${BATS_TEST_DIRNAME}/../../../test_helper.bash"
  isolate_git
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
  LIB="$ROOT/packages/agents/claude/.apm/skills/commit/scripts/lib.sh"
  export LIB
  git init --quiet --initial-branch=main "$BATS_TEST_TMPDIR/repo"
  cd "$BATS_TEST_TMPDIR/repo"
  git config user.name Tester
  git config user.email tester@example.com
  unset CLAUDE_CODE_SESSION_ID CODEX_THREAD_ID CLAUDECODE
  export XDG_CONFIG_HOME="$BATS_TEST_TMPDIR/config"
}

reject() {
  run bash -c '. "$LIB"; eval "$1"; if eval "$2"; then exit 0; else exit 42; fi' _ "$1" "$2"
  [ "$status" -eq 42 ]
}

@test "digest rejects a checksum producer that prints a digest and fails" {
  reject 'sha256sum() { printf "abc  draft\n"; return 9; }' 'dt_digest draft'
}

@test "repository name rejects failed repository discovery" {
  reject 'gitr() { printf "%s/.git\n" "$PWD"; return 9; }' 'dt_repo_name'
}

@test "state and input directories reject failed directory discovery" {
  reject 'dt_git_dir() { echo "$PWD/.git"; return 9; }' 'dt_state_dir'
  reject 'dt_state_dir() { echo "$PWD/.git"; return 9; }' 'dt_new_inputs t'
}

@test "policy freshness rejects a matching authority printed before failure" {
  reject 'DT_POLICY_REF=base; DT_POLICY_OID=oid; dt_policy_ref() { echo base; return 9; }; gitr() { echo oid; }' 'dt_policy_current'
}

@test "policy blob mode must be read successfully before policy is trusted" {
  reject '
    dt_policy_ref() { echo base; }
    gitr() {
      case "$1" in
        rev-parse) echo oid ;;
        merge-base) echo base ;;
        diff) return 0 ;;
        ls-tree) echo "100644 blob oid policy"; return 9 ;;
        *) echo policy ;;
      esac
    }
    mkdir inputs
  ' 'dt_review_policy inputs head tree'
}

@test "review prompt refuses failed input enumeration and criteria reads" {
  printf 'criteria\n' >criteria
  mkdir inputs
  reject 'find() { echo inputs/draft; return 9; }' 'dt_review_prompt criteria args inputs'
  reject 'awk() { echo criteria; return 9; }' 'dt_review_prompt criteria args inputs'
}

@test "verdict rejects failed byte readers even if their output looks valid" {
  printf 'VERDICT: PASS\n' >reply
  reject 'tr() { command tr "$@"; return 9; }' 'dt_verdict reply'
  reject 'wc() { command wc "$@"; return 9; }' 'dt_verdict reply'
}

@test "stamp validation rejects a matching digest or binding printed before failure" {
  mkdir -p .git/agent-skills
  printf 'draft abc\n' >.git/agent-skills/t.stamp
  reject 'dt_digest() { echo abc; return 9; }' 'dt_require_stamp t draft'
  reject 'dt_stamp_value() { echo abc; return 9; }; dt_digest() { echo abc; }' 'dt_require_stamp t draft'
}

@test "preapproval rejects a matching grant printed by a failed reader" {
  mkdir -p .git/preapprovals
  echo "commit granted" >.git/preapprovals/session
  reject 'dt_session_id() { echo session; }; dt_git_dir() { echo "$PWD/.git"; return 9; }' 'dt_preapproval commit'
}

@test "delegation never calls a checker with failed configuration or repository discovery" {
  printf '#!/usr/bin/env bash\ntouch called\n' >checker
  chmod +x checker
  reject 'dt_conf() { echo "$PWD/checker"; return 9; }' 'dt_delegation commit'
  [ ! -e called ]
  reject 'dt_conf() { echo "$PWD/checker"; }; dt_repo_name() { echo repo; return 9; }' 'dt_delegation commit'
  [ ! -e called ]
}

# Read only the named function from the generated executable. This lets
# freshness predicates be exercised under ! without starting a model or gh.
function_from() {
  sed -n "/^$2() {/,/^}/p" "$1" >function.sh
}

@test "all review freshness predicates reject matching output with failed status" {
  for skill in commit pr merge-pr; do
    function_from "$ROOT/packages/agents/claude/.apm/skills/$skill/scripts/review.sh" still_as_gathered
    [ -s function.sh ]
    reject '
      . ./function.sh
      tree=tree; head=head; base=base; base_ref=base; base_oid=oid
      note_digest=none; number=1
      gitr() { echo tree; return 9; }
      dt_head() { echo head; return 9; }
      dt_state_dir() { echo "$PWD"; }
      pr_base_ref() { echo base; }
      mp_view() { :; }; mp_require_reviews() { :; }
      mp_field() { case "$2" in headRefOid) echo head;; *) echo base;; esac; return 9; }
      mp_base_oid() { echo oid; }
    ' 'still_as_gathered'
  done
}

@test "marker exemptions reject documented attributes printed before git fails" {
  file="$ROOT/packages/agents/claude/.apm/skills/resolve-rebase-conflicts/scripts/guard-resolve.sh"
  function_from "$file" documented
  [ -s function.sh ]
  reject '. ./function.sh; git() { echo "file: conflict-markers: documented"; return 9; }' 'documented file'
}

@test "review refuses a failed prompt producer and leaves no stamp" {
  run bash -c '
    . "$LIB"
    mkdir inputs
    echo draft >draft; cp draft inputs/draft; echo criteria >criteria
    dt_review_prompt() { echo prompt; return 9; }
    dt_run_reviewer() { cat >/dev/null; echo "VERDICT: PASS"; }
    current() { return 0; }
    dt_review t draft inputs/draft criteria args inputs current test-subject
  '
  [ "$status" -eq 4 ]
  [ ! -e .git/agent-skills/t.stamp ]
}

@test "review rejects a digest failure after a passing reviewer" {
  run bash -c '
    . "$LIB"
    mkdir inputs
    echo draft >draft; cp draft inputs/draft; echo criteria >criteria
    dt_digest() { echo abc; [ ! -f reviewed ]; }
    dt_run_reviewer() { cat >/dev/null; touch reviewed; echo "VERDICT: PASS"; }
    current() { return 0; }
    dt_review t draft inputs/draft criteria args inputs current test-subject
  '
  [ "$status" -eq 5 ]
  [ ! -e .git/agent-skills/t.stamp ]
}

@test "commit and PR publication guards reject failed state producers before confirmation" {
  for skill in commit pr; do
    script=commit.sh
    [ "$skill" != pr ] || script=create-pr.sh
    function_from "$ROOT/packages/agents/claude/.apm/skills/$skill/scripts/$script" require_reviewed
    [ -s function.sh ]
    mkdir -p state
    touch "state/$skill.stamp"
    reject '
      . ./function.sh
      state=state; draft=draft; mode=normal; here=here
      fail() { exit 1; }; die() { exit 1; }
      dt_require_stamp() { return 0; }
      gitr() { echo tree; return 9; }
      dt_head() { echo head; return 9; }
      pr_field() { echo base; return 9; }
      pr_base_ref() { echo base; }
    ' 'require_reviewed'
  done
}

@test "stamp writer propagates a failed binding formatter" {
  reject 'tr() { echo value; return 9; }' '_dt_stamp stamp digest key value'
}

@test "stamp lookup preserves a failed reader status" {
  mkdir -p .git/agent-skills
  echo 'draft abc' >.git/agent-skills/t.stamp
  reject 'sed() { echo abc; return 9; }' 'dt_stamp_value t draft'
}

@test "merge diff refuses failed shallow-repository detection" {
  function_from "$ROOT/packages/agents/claude/.apm/skills/merge-pr/scripts/pull-request.sh" mp_local_diff
  reject '
    . ./function.sh
    dt_gh() { echo repository; }
    gitr() { if [ "$1" = rev-parse ]; then echo false; return 9; fi; touch fetched; }
  ' 'mp_local_diff base head inputs'
  [ ! -e fetched ]
}

@test "retrospective creation rejects metadata printed before parser failure" {
  mkdir -p skill/scripts skill/templates
  cp "$ROOT/packages/agents/claude/.apm/skills/analyze-session/scripts/new-retro.sh" skill/scripts/
  cp "$LIB" skill/scripts/lib.sh
  cp "${LIB%/*}/agent.sh" "${LIB%/*}/signing.sh" skill/scripts/
  echo template >skill/templates/RETRO.md
  cat >skill/scripts/session.py <<'STUB'
#!/usr/bin/env bash
printf 'SESSION_ID=session\nSESSION_DATE=2026-01-02\nRECORDS=1\nSIZE_MB=1\n'
[ "$1" != meta ] || exit 9
STUB
  chmod +x skill/scripts/session.py
  echo transcript >transcript.jsonl
  run bash skill/scripts/new-retro.sh transcript.jsonl
  [ "$status" -eq 9 ]
  [ ! -d .claude/retros ]
  [ ! -d .codex/retros ]
}

@test "dictionary verification rejects diff errors instead of treating silence as sanctioned" {
  function_from "$ROOT/packages/agents/claude/.apm/skills/fix-prose/scripts/check-suppressions.sh" unsanctioned_changes
  mkdir -p state/watched
  echo before >state/watched/dictionary
  echo after >dictionary
  reject '. ./function.sh; TAB=$(printf "\t"); diff() { return 2; }' 'unsanctioned_changes state dictionary'
}
