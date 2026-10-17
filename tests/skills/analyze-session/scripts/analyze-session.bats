#!/usr/bin/env bats
# The analyze-session skill's scripts, as generated. session.py reads two
# transcript formats behind one command line, and these cases run it against
# a small synthetic transcript of each. They also run each harness package's
# copy against the other harness's format, because the parser follows the
# file and not the package it was deployed from. The retro cases run in a
# scratch repository under a scratch HOME with the session variables cleared.

setup() {
  source "${BATS_TEST_DIRNAME}/../../../test_helper.bash"
  isolate_git
  ROOT="${BATS_TEST_DIRNAME}/../../../.."
  CLAUDE_SCRIPTS="${ROOT}/packages/agents/claude/.apm/skills/analyze-session/scripts"
  CODEX_SCRIPTS="${ROOT}/packages/agents/codex/.apm/skills/analyze-session/scripts"
  unset CLAUDE_CODE_SESSION_ID CODEX_THREAD_ID CLAUDECODE DEVTOOLS_HARNESS
  unset ANALYZE_SESSION_FORMAT
  export HOME="${BATS_TEST_TMPDIR}/home"
  export XDG_CONFIG_HOME="${BATS_TEST_TMPDIR}/config"
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
  mkdir -p "$HOME"
  CLAUDE_T="${HOME}/.claude/projects/-work-proj/session-claude-1.jsonl"
  CODEX_T="${HOME}/.codex/sessions/2026/01/02/rollout-2026-01-02T12-00-00-019abc00-0000-0000-0000-00000000cafe.jsonl"
  mkdir -p "$(dirname "$CLAUDE_T")" "$(dirname "$CODEX_T")"
  claude_fixture >"$CLAUDE_T"
  codex_fixture >"$CODEX_T"
  REPO="${BATS_TEST_TMPDIR}/repo"
  git init --quiet --initial-branch=main "$REPO"
}

# Three lint rounds in a Bash call, one of them blocked by a hook, then a
# permission refusal.
claude_fixture() {
  printf '%s\n' \
    '{"type":"user","timestamp":"2026-01-02T12:00:00Z","message":{"role":"user","content":"lint it"}}' \
    '{"type":"assistant","timestamp":"2026-01-02T12:00:01Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"try the linter"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"mise run lint-prose"}}]}}' \
    '{"type":"user","timestamp":"2026-01-02T12:00:02Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"Exit code 1\nfile.md:1 [error] AI.Hedge match=\"just\""}]}}' \
    '{"type":"assistant","timestamp":"2026-01-02T12:00:03Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"mise run lint-prose"}}]}}' \
    '{"type":"user","timestamp":"2026-01-02T12:00:04Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t2","content":"PreToolUse hook error: blocked"}]}}' \
    '{"type":"assistant","timestamp":"2026-01-02T12:00:05Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t3","name":"Bash","input":{"command":"mise run lint-prose"}}]}}' \
    '{"type":"user","timestamp":"2026-01-02T12:00:06Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t3","is_error":true,"content":"The user has been denied this command"}]}}'
}

# The same story as a Codex rollout: nested exec_command calls.
codex_fixture() {
  printf '%s\n' \
    '{"type":"session_meta","timestamp":"2026-01-02T12:00:00Z","payload":{"id":"019abc00-feed","cwd":"/work/proj"}}' \
    '{"type":"response_item","timestamp":"2026-01-02T12:00:01Z","payload":{"type":"reasoning","summary":[{"text":"try the linter"}]}}' \
    '{"type":"response_item","timestamp":"2026-01-02T12:00:02Z","payload":{"type":"custom_tool_call","name":"exec","input":"await tools.exec_command({cmd: \"mise run lint-prose\"})"}}' \
    '{"type":"response_item","timestamp":"2026-01-02T12:00:03Z","payload":{"type":"custom_tool_call_output","output":"Script failed with code 1\nfile.md:1 [error] AI.Hedge match=\"just\""}}' \
    '{"type":"response_item","timestamp":"2026-01-02T12:00:04Z","payload":{"type":"custom_tool_call","name":"exec","input":"await tools.exec_command({cmd: \"mise run lint-prose\"})"}}' \
    '{"type":"response_item","timestamp":"2026-01-02T12:00:05Z","payload":{"type":"custom_tool_call_output","output":"Script failed with code 1\nPermission denied"}}' \
    '{"type":"response_item","timestamp":"2026-01-02T12:00:06Z","payload":{"type":"custom_tool_call","name":"exec","input":"await tools.exec_command({cmd: \"mise run lint-prose\"})"}}' \
    '{"type":"response_item","timestamp":"2026-01-02T12:00:07Z","payload":{"type":"custom_tool_call_output","output":"ok"}}'
}

@test "index detects a Claude transcript and counts its Bash calls" {
  run "$CLAUDE_SCRIPTS/session.py" index "$CLAUDE_T"

  [ "$status" -eq 0 ]
  [[ "$output" == *"format:         claude"* ]]
  [[ "$output" == *"3  Bash"* ]]
  [[ "$output" == *"failing results: 3"* ]]
}

@test "index detects a Codex rollout and counts its exec calls" {
  run "$CODEX_SCRIPTS/session.py" index "$CODEX_T"

  [ "$status" -eq 0 ]
  [[ "$output" == *"format:         codex"* ]]
  [[ "$output" == *"3  exec"* ]]
  [[ "$output" == *"failing results: 2"* ]]
}

@test "the Codex package's copy analyzes a Claude transcript, and the reverse" {
  run "$CODEX_SCRIPTS/session.py" index "$CLAUDE_T"
  [ "$status" -eq 0 ]
  [[ "$output" == *"format:         claude"* ]]
  [[ "$output" == *"3  Bash"* ]]

  run "$CLAUDE_SCRIPTS/session.py" index "$CODEX_T"
  [ "$status" -eq 0 ]
  [[ "$output" == *"format:         codex"* ]]
  [[ "$output" == *"3  exec"* ]]
}

@test "the two packages ship the same session.py" {
  cmp "$CLAUDE_SCRIPTS/session.py" "$CODEX_SCRIPTS/session.py"
}

@test "--format overrides detection" {
  run "$CLAUDE_SCRIPTS/session.py" index "$CLAUDE_T" --format codex

  [ "$status" -eq 0 ]
  [[ "$output" == *"format:         codex"* ]]
  [[ "$output" == *"failing results: 0"* ]]
}

@test "a file in neither format is refused rather than parsed with a guess" {
  printf '%s\n' '{"hello":"world"}' >"${BATS_TEST_TMPDIR}/odd.jsonl"

  run "$CLAUDE_SCRIPTS/session.py" index "${BATS_TEST_TMPDIR}/odd.jsonl"

  [ "$status" -eq 1 ]
  [[ "$output" == *"pass --format claude|codex"* ]]
}

@test "errors classifies hook blocks and denials in a Claude transcript" {
  run "$CLAUDE_SCRIPTS/session.py" errors "$CLAUDE_T"

  [ "$status" -eq 0 ]
  [[ "$output" == *"[lint]"* ]]
  [[ "$output" == *"[hook-block]"* ]]
  [[ "$output" == *"[permission]"* ]]
  [[ "$output" == *"3 failing result(s)"* ]]
}

@test "errors classifies a Codex failure envelope and a permission error" {
  run "$CLAUDE_SCRIPTS/session.py" errors "$CODEX_T"

  [ "$status" -eq 0 ]
  [[ "$output" == *"[lint]"* ]]
  [[ "$output" == *"[permission]"* ]]
  [[ "$output" == *"2 failing result(s)"* ]]
}

@test "loops groups repeated calls under each format's own signature" {
  run "$CLAUDE_SCRIPTS/session.py" loops "$CLAUDE_T"
  [ "$status" -eq 0 ]
  [[ "$output" == *"3 rounds"*"mise run lint-prose"* ]]

  run "$CLAUDE_SCRIPTS/session.py" loops "$CODEX_T"
  [ "$status" -eq 0 ]
  [[ "$output" == *"3 rounds"*"mise run lint-prose"* ]]
}

@test "grep reads decoded thinking from either format" {
  run "$CLAUDE_SCRIPTS/session.py" grep "$CLAUDE_T" 'try the' --kind THINK
  [ "$status" -eq 0 ]
  [[ "$output" == *"1 match(es)"* ]]

  run "$CLAUDE_SCRIPTS/session.py" grep "$CODEX_T" 'try the' --kind THINK
  [ "$status" -eq 0 ]
  [[ "$output" == *"1 match(es)"* ]]
}

@test "meta takes the session id from the filename for Claude and the metadata for Codex" {
  run "$CLAUDE_SCRIPTS/session.py" meta "$CLAUDE_T"
  [ "$status" -eq 0 ]
  [[ "$output" == *"FORMAT=claude"* ]]
  [[ "$output" == *"SESSION_ID=session-claude-1"* ]]

  run "$CLAUDE_SCRIPTS/session.py" meta "$CODEX_T"
  [ "$status" -eq 0 ]
  [[ "$output" == *"FORMAT=codex"* ]]
  [[ "$output" == *"SESSION_ID=019abc00-feed"* ]]
}

@test "locate searches both stores and names each hit's format" {
  run "$CLAUDE_SCRIPTS/session.py" locate --contains 'mise run lint-prose'

  [ "$status" -eq 0 ]
  [[ "$output" == *"session-claude-1.jsonl  matches=3  format=claude"* ]]
  [[ "$output" == *"cafe.jsonl  matches=3  format=codex"* ]]
}

@test "locate --project matches a Claude project directory and a Codex cwd" {
  run "$CLAUDE_SCRIPTS/session.py" locate --project proj

  [ "$status" -eq 0 ]
  [[ "$output" == *"format=claude"* ]]
  [[ "$output" == *"format=codex"* ]]

  run "$CLAUDE_SCRIPTS/session.py" locate --project absent
  [[ "$output" == *"no transcript matched"* ]]
}

@test "session.py has no bare multi-exception clause" {
  # A bare `except A, B:` is valid only from Python 3.14. The uv header
  # guarantees 3.14 for a shebang run, but not for python3 session.py.
  run grep -nE '^[[:space:]]*except [A-Za-z_.]+, ' "$CLAUDE_SCRIPTS/session.py"
  [ "$status" -eq 1 ]
}

@test "new-retro puts the ignore entry in .git/info/exclude and leaves .gitignore alone" {
  cd "$REPO"

  run "$CLAUDE_SCRIPTS/new-retro.sh" "$CLAUDE_T"

  [ "$status" -eq 0 ]
  [[ "$output" == *"gitignore:  added to .git/info/exclude"* ]]
  grep -qxF '.claude/retros/' .git/info/exclude
  [ ! -e .gitignore ]
  [ -z "$(git status --porcelain)" ]
  [ -f .claude/retros/20260102_session-claude-1/RETRO.md ]
  [ -f .claude/retros/20260102_session-claude-1/artifacts/index.txt ]
}

@test "new-retro files a Codex session's retro under .codex when Codex runs it" {
  cd "$REPO"
  export DEVTOOLS_HARNESS=codex

  run "$CODEX_SCRIPTS/new-retro.sh" "$CODEX_T"

  [ "$status" -eq 0 ]
  grep -qxF '.codex/retros/' .git/info/exclude
  [ -f .codex/retros/20260102_019abc00-feed/RETRO.md ]
}

@test "new-retro run twice keeps one exclude entry and the author's RETRO.md" {
  cd "$REPO"
  "$CLAUDE_SCRIPTS/new-retro.sh" "$CLAUDE_T" >/dev/null
  printf 'mine\n' >.claude/retros/20260102_session-claude-1/RETRO.md

  run "$CLAUDE_SCRIPTS/new-retro.sh" "$CLAUDE_T"

  [ "$status" -eq 0 ]
  [[ "$output" == *"gitignore:  already ignored"* ]]
  [ "$(grep -cxF '.claude/retros/' .git/info/exclude)" -eq 1 ]
  [ "$(cat .claude/retros/20260102_session-claude-1/RETRO.md)" = mine ]
}

@test "preflight resolves a bare id from either store and reports where the retro went" {
  cd "$REPO"

  run "$CLAUDE_SCRIPTS/preflight.sh" session-claude-1
  [ "$status" -eq 0 ]
  [[ "$output" == *"$CLAUDE_T"* ]]
  [[ "$output" == *"format:         claude"* ]]
  [ -d .claude/retros/20260102_session-claude-1/artifacts ]

  run "$CLAUDE_SCRIPTS/preflight.sh" 019abc00-0000-0000-0000-00000000cafe
  [ "$status" -eq 0 ]
  [[ "$output" == *"$CODEX_T"* ]]
  [[ "$output" == *"format:         codex"* ]]
}

@test "preflight stops with instructions when nothing resolves" {
  cd "$REPO"

  run "$CLAUDE_SCRIPTS/preflight.sh" no-such-session

  [ "$status" -eq 0 ]
  [[ "$output" == *"UNRESOLVED"* ]]
  [[ "$output" == *"~/.claude/projects"* ]]
  [[ "$output" == *"~/.codex/sessions"* ]]
}

@test "preflight rejects a basename printed before its command fails" {
  export REAL_BASENAME
  REAL_BASENAME=$(command -v basename)
  mkdir -p "$BATS_TEST_TMPDIR/bin"
  cat >"$BATS_TEST_TMPDIR/bin/basename" <<'STUB'
#!/usr/bin/env bash
"$REAL_BASENAME" "$@" || exit $?
exit 9
STUB
  chmod +x "$BATS_TEST_TMPDIR/bin/basename"
  cd "$REPO"
  PATH="$BATS_TEST_TMPDIR/bin:$PATH" run bash "$CLAUDE_SCRIPTS/preflight.sh" "$CLAUDE_T"
  [ "$status" -eq 9 ]
  [ ! -d .claude/retros ]
}
