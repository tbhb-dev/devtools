#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_pr_setup
}

@test "agent PR publishes reviewed bytes with a structured result and resets rounds" {
  grant
  mkdir -p .git/agent-skills
  printf '2\n' >.git/agent-skills/pr.rounds
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 0 ]
  [[ "$output" == *'published: https://example.com/pull/1 at '* ]]
  [ "$(jq -r .head .git/agent-skills/pr.result.json)" = "$(git rev-parse HEAD)" ]
  [ "$(jq -r .draft .git/agent-skills/pr.result.json)" = false ]
  [ ! -e .git/agent-skills/pr.rounds ]
  [ ! -e .git/agent-skills/pr.stamp ]
  grep -q 'pr create .*--body-file' "$GH_LOG"
}

@test "agent PR updates properties and draft state through shared publication" {
  grant
  export GH_OPEN_PR=1
  replace_text PR_AGENTDESC.md 'draft: false' 'draft: true'
  replace_text PR_AGENTDESC.md 'labels: [enhancement]' 'labels: [enhancement, bug]'
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 0 ]
  grep -q 'pr edit 1 --add-label enhancement' "$GH_LOG"
  grep -q 'pr ready 1 --undo' "$GH_LOG"
  [ "$(jq -r .draft .git/agent-skills/pr.result.json)" = true ]
}

@test "agent PR reporting is configured data and failed delivery never republishes" {
  grant
  export MAIL_ARGS="$BATS_TEST_TMPDIR/mail.args"
  cat >"$BATS_TEST_TMPDIR/mail" <<'MAIL'
#!/usr/bin/env bash
printf '%s\n' "$@" >"$MAIL_ARGS"
exit 1
MAIL
  chmod +x "$BATS_TEST_TMPDIR/mail"
  printf 'mail_cmd = %s/mail\nmail_to = test recipient\nmail_from = reporter\n' "$BATS_TEST_TMPDIR" >.agents/agent-skills.conf
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 0 ]
  [[ "$output" == *'REPORT NOT SENT'* ]]
  [ "$(grep -c '^pr create ' "$GH_LOG")" -eq 1 ]
  grep -qx 'test recipient' "$MAIL_ARGS"
  grep -qx reporter "$MAIL_ARGS"
  [ -f .git/agent-skills/pr.result.json ]
}

@test "agent PR refuses a changed draft or branch after the push" {
  grant
  printf '#!/usr/bin/env bash\necho changed >>PR_AGENTDESC.md\n' >.git/hooks/pre-push
  chmod +x .git/hooks/pre-push
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 1 ]
  [ "$(remote_topic)" = "$(git rev-parse HEAD)" ]
  refute_grep '^pr create ' "$GH_LOG"
}

@test "agent PR retains the shared non-fast-forward refusal and exact lease" {
  grant
  bash "$SCRIPTS/review.sh"
  bash "$SCRIPTS/publish.sh"
  git commit --amend -qm 'feat: replace the widget'
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'not a fast-forward'* ]]
  run bash "$SCRIPTS/publish.sh" --rewrite
  [ "$status" -eq 0 ]
  [ "$(jq -r .push .git/agent-skills/pr.result.json)" = rewrite ]
  [ "$(remote_topic)" = "$(git rev-parse HEAD)" ]
}

@test "agent PR rechecks the account after a pre-push hook" {
  grant
  cat >.git/hooks/pre-push <<'HOOK'
#!/usr/bin/env bash
python3 - <<'PYTHON'
from pathlib import Path
import os
file = Path(os.environ['BATS_TEST_TMPDIR']) / 'bin/gh'
file.write_text(file.read_text().replace('--arg login "$GH_USER"', '--arg login rogue'))
PYTHON
HOOK
  chmod +x .git/hooks/pre-push
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'not agent'* ]]
  refute_grep '^pr create ' "$GH_LOG"
}

@test "agent PR report resolves a numeric branch by head instead of PR number" {
  grant
  git branch -m 123
  bash "$SCRIPTS/review.sh"
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 0 ]
  [ "$(jq -r .number .git/agent-skills/pr.result.json)" -eq 1 ]
  grep -q 'pr list --head 123 --state open --json number,url' "$GH_LOG"
}

@test "description and property edits reuse test and lint on publish dry runs (#555)" {
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; echo review >>"$GATE_LOG"; echo "VERDICT: PASS"'
  bash "$SCRIPTS/review.sh"
  bash "$SCRIPTS/publish.sh" --dry-run
  for change in description properties; do
    case "$change" in
    description) printf '\nEdited description.\n' >>PR_AGENTDESC.md ;;
    properties) replace_text PR_AGENTDESC.md 'labels: [enhancement]' 'labels: [enhancement, bug]' ;;
    esac
    bash "$SCRIPTS/review.sh"
    : >"$GATE_LOG"
    run bash "$SCRIPTS/publish.sh" --dry-run
    [ "$status" -eq 0 ]
    [[ "$output" != *"mise run lint passed at"* ]]
    [[ "$output" == *"mise run test-affected"*"(reused)"* ]]
    [ "$(cat "$GATE_LOG")" = $'lint-pr-description\ndevtools:lint-leaks\ndevtools:gitleaks\nlint\nreview' ]
  done
}

@test "agent PR without either base ref exits 2 before any push (#651)" {
  git update-ref -d refs/remotes/origin/main
  git branch -D main
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 2 ]
  [[ "$output" == *"origin/main is missing; the leak scan is incomplete. Nothing was pushed."* ]]
  [ -z "$(remote_topic)" ]
  refute_grep '^devtools:lint-leaks' "$GATE_LOG"
  refute_grep '^pr create ' "$GH_LOG"
}
