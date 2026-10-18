#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  agent_pr_setup
}

@test "agent PR custom lint runs and an empty lint command refuses" {
  printf 'lint_pr_cmd = printf configured >"$BATS_TEST_TMPDIR/lint-ran"\n' >.agents/agent-skills.conf
  bash "$SCRIPTS/check.sh"
  [ "$(cat "$BATS_TEST_TMPDIR/lint-ran")" = configured ]
  echo 'lint_pr_cmd =' >.agents/agent-skills.conf
  run bash "$SCRIPTS/check.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'lint_pr_cmd must not be empty'* ]]
}

@test "agent PR log and Related grammar reject invalid prose" {
  printf 'pr_log_entry = required\npr_related_pattern = ^(#[0-9]+|R[0-9]+)$\n' >.agents/agent-skills.conf
  run bash "$SCRIPTS/check.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'must end with Log entry'* ]]
  [[ "$output" == *'Related must contain'* ]]
  printf '\n## Related\n\n#12, R9\n\nLog entry: * **Feature**: Add the widget.\n' >>PR_AGENTDESC.md
  cp .github/pull_request_template.md .github/custom.md
  printf '\n## Related\n' >>.github/custom.md
  git add -- .github/custom.md
  git commit -qm 'docs: add a template'
  echo 'pr_template = .github/custom.md' >>.agents/agent-skills.conf
  bash "$SCRIPTS/check.sh"
  replace_text PR_AGENTDESC.md '#12, R9' 'None, #12'
  run bash "$SCRIPTS/check.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'invalid item'* ]]
}

@test "agent PR title command receives the resolved range and exact title" {
  cat >.agents/agent-skills.conf <<'CONF'
pr_title_check_cmd = test "$AS_BASE" = origin/main && test "$AS_HEAD" = HEAD && test "$AS_TITLE" = 'feat: add the widget file'
CONF
  bash "$SCRIPTS/check.sh"
  echo 'pr_title_check_cmd = false' >.agents/agent-skills.conf
  run bash "$SCRIPTS/check.sh"
  [ "$status" -ne 0 ]
}

@test "configured template is shared by scaffold validator and review" {
  printf '## Custom\n\n' >.github/custom.md
  git add -- .github/custom.md
  git commit -qm 'docs: add a custom template'
  echo 'pr_template = .github/custom.md' >.agents/agent-skills.conf
  rm PR_AGENTDESC.md
  bash "$ROOT/packages/agents/codex/.apm/skills/pr/scripts/scaffold-description.sh"
  grep -q '^## Custom' PR_AGENTDESC.md
  replace_text PR_AGENTDESC.md 'labels: []' 'labels: [enhancement]'
  replace_text PR_AGENTDESC.md '# <type>(<scope>)?: <description>' '# feat: add the widget file'
  printf '\nCustom prose.\n' >>PR_AGENTDESC.md
  bash "$SCRIPTS/check.sh"
  bash "$SCRIPTS/review.sh"
  template=$(find .git/agent-skills -name pull_request_template.md -print -quit)
  cmp .github/custom.md "$template"
}

@test "agent PR config stays data and cannot choose authorities" {
  printf 'mail_to = $(touch injected)\n' >.agents/agent-skills.conf
  bash "$SCRIPTS/check.sh"
  [ ! -e injected ]
  echo 'gh_login = forbidden' >>.agents/agent-skills.conf
  run bash "$SCRIPTS/check.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'Invalid or duplicate repository setting'* ]]
}

@test "agent PR config drift voids a stamp" {
  grant
  bash "$SCRIPTS/review.sh"
  echo 'pr_log_entry = optional' >.agents/agent-skills.conf
  run bash "$SCRIPTS/publish.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'configuration changed'* ]]
  [ -z "$(remote_topic)" ]
}

@test "agent PR branch switching during review prevents stamping" {
  git branch alternate
  export DEVTOOLS_REVIEW_CMD='cat >/dev/null; git switch -q alternate; echo "VERDICT: PASS"'
  run bash "$SCRIPTS/review.sh"
  [ "$status" -eq 5 ]
  [ ! -e .git/agent-skills/pr.stamp ]
}

@test "invalid Related regex refuses even when the section says None" {
  printf 'pr_related_pattern = [\n' >.agents/agent-skills.conf
  printf '\n## Related\n\nNone\n' >>PR_AGENTDESC.md
  cp .github/pull_request_template.md .github/custom.md
  printf '\n## Related\n' >>.github/custom.md
  git add -- .github/custom.md
  git commit -qm 'docs: add a template'
  echo 'pr_template = .github/custom.md' >>.agents/agent-skills.conf
  run bash "$SCRIPTS/check.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'Invalid pr_related_pattern'* ]]
}

@test "agent PR refuses ignored or symlink templates that snapshots cannot read" {
  printf '.github/ignored.md\n' >>.git/info/exclude
  cp .github/pull_request_template.md .github/ignored.md
  echo 'pr_template = .github/ignored.md' >.agents/agent-skills.conf
  run bash "$SCRIPTS/review.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'committed regular file'* ]]
  ln -s pull_request_template.md .github/alias.md
  git add -- .github/alias.md
  git commit -qm 'docs: add a template link'
  echo 'pr_template = .github/alias.md' >.agents/agent-skills.conf
  run bash "$SCRIPTS/review.sh"
  [ "$status" -eq 1 ]
  [[ "$output" == *'committed regular file'* ]]
}
