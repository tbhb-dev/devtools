#!/usr/bin/env bash
# pull-request — readers of a pull request's live state, shared by the
# merge-pr scripts that judge it: preflight.sh, review.sh, and
# squash-merge.sh.
#
# Sourced, never run, after lib.sh. Written for bash 3.2. Every reader
# here works from one `gh pr view --json` response saved to a file, so
# the facts a script judges together all come from the same moment.

# --- environment hardening -------------------------------------------
# lib.sh and the sourcing script repeat this block; it stays here too so
# the hygiene gate can see it in the file that calls gh.
export LC_ALL=C
export GH_PAGER=cat
export GH_PROMPT_DISABLED=1
export PYTHONUTF8=1
unset CDPATH GH_REPO GH_HOST GREP_OPTIONS

# mp_need_tools: gh, signed in, and jq, or stop.
mp_need_tools() {
  command -v gh >/dev/null 2>&1 || dt_die "gh is not installed"
  gh auth status >/dev/null 2>&1 || dt_die "gh is not authenticated; run gh auth login"
  command -v jq >/dev/null 2>&1 || dt_die "jq is not installed"
}

# mp_view <number> <file> <field,...>: save one response naming every
# field the caller reads.
mp_view() {
  gh pr view "$1" --json "$3" >"$2"
}

# mp_field <file> <field>: one top-level field of a saved response, a
# boolean as true or false, a missing value as nothing.
mp_field() {
  jq -r --arg f "$2" '.[$f] | if . == null then "" else tostring end' "$1"
}

# mp_base_oid <base-ref>: the commit the base branch points at on
# GitHub, read through gh so it names the same repository gh merges in.
mp_base_oid() {
  gh api "repos/{owner}/{repo}/git/ref/heads/$1" --jq .object.sha
}

# mp_judge_checks <file>: judge the statusCheckRollup in a saved
# response, printing one line a check: a verdict (ok, pending, or fail),
# the state it read, and the check's name, separated by tabs.
#
# The rollup keeps every run GitHub started for the head commit, not
# only the latest. A workflow whose concurrency group cancelled a first
# run when a second began leaves the cancelled jobs in the rollup beside
# the passing ones, and counting those would refuse a green pull
# request. So check runs are grouped by workflow and check name, and the
# one that started last stands for its group. A run that has not started
# has no start time and counts as the latest, so a queued re-run reads
# as pending rather than letting the run it replaces decide. A cancel
# with no later run still fails. Status contexts carry no workflow or
# run history of the same shape, so each one is judged as it stands.
#
#   0  every check ok      1  a check failed      2  a check pending
#   3  the rollup could not be read
mp_judge_checks() {
  local out
  out=$(jq -r '
    def is_status: (.__typename // "") == "StatusContext"
      or ((.name // null) == null and (.context // null) != null);
    def started:
      (.startedAt // "") | if . == "" or startswith("0001") then "9999-12-31T23:59:59Z" else . end;
    def state:
      if is_status then ((.state // "PENDING") | ascii_upcase)
      elif ((.status // "COMPLETED") | ascii_upcase) != "COMPLETED" then (.status | ascii_upcase)
      else ((.conclusion // "") | ascii_upcase | if . == "" then "PENDING" else . end)
      end;
    def verdict:
      if test("^(FAILURE|TIMED_OUT|CANCELLED|ACTION_REQUIRED|ERROR|STARTUP_FAILURE|STALE)$") then "fail"
      elif test("^(PENDING|IN_PROGRESS|QUEUED|EXPECTED|WAITING|REQUESTED)$") then "pending"
      else "ok" end;
    def check_name:
      if is_status then .context
      else (((.workflowName // "") | if . == "" then "" else . + " / " end) + (.name // "")) end;
    (.statusCheckRollup // []) as $all
    | ($all | map(select(is_status | not))
        | group_by([(.workflowName // ""), (.name // "")])
        | map(max_by(started))) as $latest
    | ($latest + ($all | map(select(is_status))))[]
    | state as $s
    | [($s | verdict), $s, check_name]
    | @tsv
  ' "$1" 2>/dev/null) || return 3
  [ -z "$out" ] || printf '%s\n' "$out"
  printf '%s\n' "$out" | awk -F'\t' '
    $1 == "fail" { failed = 1 }
    $1 == "pending" { pending = 1 }
    END { exit failed ? 1 : (pending ? 2 : 0) }
  '
}

# mp_superseded <file>: how many check runs the judgement left out
# because a later run of the same check replaced them.
mp_superseded() {
  jq -r '
    (.statusCheckRollup // [])
    | map(select((.__typename // "") != "StatusContext" and (.name // null) != null))
    | length - (group_by([(.workflowName // ""), .name]) | length)
  ' "$1"
}
