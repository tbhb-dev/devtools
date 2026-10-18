#!/usr/bin/env bats
# Live harness probes are opt-in: normal test runs require no model calls.
setup() {
  source "${BATS_TEST_DIRNAME}/../../test_helper.bash"
  isolate_git
  ROOT=$(cd "${BATS_TEST_DIRNAME}/../../.." && pwd)
  SKILLS="$ROOT/packages/agents/claude/.apm/skills"
}

@test "writer manifests allow their context commands without a prompt (#137)" {
  for writer in write-pr-description write-prose-fix; do
    manifest="$SKILLS/$writer/SKILL.md"
    grep -qx 'allowed-tools:' "$manifest"
    grep -Fxq '  - Bash(bash ${CLAUDE_SKILL_DIR}/scripts/context.sh*)' "$manifest"
    grep -qx '  - Edit' "$manifest"
    grep -qx '  - Write' "$manifest"
    if grep -Eq '^  - Bash(\(\*\))?$' "$manifest"; then return 1; fi
  done
  grep -Fxq '  - Bash(bash */pr/scripts/validate-description.sh*)' "$SKILLS/write-pr-description/SKILL.md"
  grep -Fxq '  - Bash(mise run lint-*)' "$SKILLS/write-prose-fix/SKILL.md"
  grep -Fxq '  - Bash(mise run fix-prose-replacements *)' "$SKILLS/write-prose-fix/SKILL.md"
}

live_fixture() {
  [ "${RUN_HEADLESS_WRITERS:-0}" = 1 ] || skip 'set RUN_HEADLESS_WRITERS=1 for authenticated claude probes'
  command -v claude >/dev/null
  unset CLAUDECODE CLAUDE_CODE_SESSION_ID CODEX_THREAD_ID
  mkdir -p "$BATS_TEST_TMPDIR/repo/.claude/skills" "$BATS_TEST_TMPDIR/bin"
  cp -R "$SKILLS/write-pr-description" "$SKILLS/write-prose-fix" "$SKILLS/pr" "$BATS_TEST_TMPDIR/repo/.claude/skills/"
  cd "$BATS_TEST_TMPDIR/repo"
  git init --quiet --initial-branch=main
  git config user.name Tester
  git config user.email tester@example.com
  mkdir -p .github .fix-prose/run.probe
  printf '## Summary\n\n## Why\n\n## Verification\n' >.github/pull_request_template.md
  printf '.claude/\n.fix-prose/\nPR_AGENTDESC.md\n' >.gitignore
  printf 'Do not remove the widget.\n' >probe.md
  printf 'probe.md\n' >.fix-prose/run.probe/target
  git add -- .github .gitignore probe.md
  git commit --quiet -m 'chore: initial state'
  git checkout --quiet -b topic
  printf 'widget\n' >widget.txt
  git add -- widget.txt
  git commit --quiet -m 'feat: add the widget'
  # Keep context gathering offline and log real tool execution independently
  # of the model's final claim. Fixture linters require the intended edit.
  export PROBE_LOG="$BATS_TEST_TMPDIR/tools.log"
  cat >"$BATS_TEST_TMPDIR/bin/gh" <<'STUB'
#!/usr/bin/env bash
exit 1
STUB
  cat >"$BATS_TEST_TMPDIR/bin/vale" <<'STUB'
#!/usr/bin/env bash
case "$*" in
*--path=*) echo '1:1-6 [error] prose match="robust" msg="Remove praise."'; exit 1 ;;
esac
STUB
  chmod +x "$BATS_TEST_TMPDIR/bin/"*
  export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
  # Claude's shell can rebuild PATH; use a real, trusted mise task for the
  # writer's lint command rather than relying on a PATH stub there.
  cat >mise.toml <<'CONFIG'
[tasks.lint-draft]
run = 'bash lint.sh'
CONFIG
  cat >lint.sh <<'LINT'
#!/usr/bin/env bash
printf '%s\n' 'run lint-draft probe.md' >>"$PROBE_LOG"
if grep -q 'Do not' probe.md; then
  echo '1:1-6 [error] Google.Contractions match="Do not" msg="Use a contraction."'
  exit 1
fi
LINT
  export MISE_TRUSTED_CONFIG_PATHS="$PWD" MISE_QUIET=1
}

probe() {
  # Project-only settings exclude personal allowlists and permission bypasses.
  # A delegated writer cannot answer prompts: the CLI must deny them.
  claude -p --setting-sources project --permission-mode default \
    --permission-prompts none --allowedTools 'Read,Glob,Grep,Skill,Agent' \
    --output-format json "$1" >"$BATS_TEST_TMPDIR/result.json"
  jq -e '.is_error == false and (.permission_denials | length == 0)' "$BATS_TEST_TMPDIR/result.json"
}

@test "headless delegated PR writer gathers context, edits, and validates (#137)" {
  live_fixture
  probe 'Invoke write-pr-description using Skill. Draft the description for this branch. No GitHub access is available; use labels: [enhancement]. Follow the skill through validation and return its DRAFT result. Do not bypass a permission denial.'
  [ -s PR_AGENTDESC.md ]
  bash .claude/skills/pr/scripts/validate-description.sh
  jq -e '.result | contains("DRAFT: WRITTEN")' "$BATS_TEST_TMPDIR/result.json"
}

@test "headless delegated prose writer gathers context, edits, and lints (#137)" {
  live_fixture
  probe 'Invoke write-prose-fix using Skill with arguments: probe.md .fix-prose/run.probe mise run lint-draft probe.md. Follow it through editing and linting, then return its PROSE result. Do not bypass a permission denial.'
  if grep -q 'Do not' probe.md; then return 1; fi
  grep -qx 'run lint-draft probe.md' "$PROBE_LOG"
  jq -e '.result | contains("PROSE: CLEAN")' "$BATS_TEST_TMPDIR/result.json"
}

@test "context gathering runs as a skill step rather than at load time (#225)" {
  for manifest in "$SKILLS"/*/SKILL.md "$ROOT"/.claude/skills/*/SKILL.md; do
    if grep -Eq '^!`.*context\.sh' "$manifest"; then return 1; fi
  done
  for writer in write-pr-description write-prose-fix; do
    grep -Eq '^bash .*scripts/context\.sh' "$SKILLS/$writer/SKILL.md"
  done
  grep -Fq 'bash ${CLAUDE_SKILL_DIR}/scripts/context.sh $ARGUMENTS' "$ROOT/.claude/skills/review-release-version/SKILL.md"
}

@test "PR workflows allow the session to draft on both harnesses (#225)" {
  for harness in claude codex; do
    for workflow in pr fix-pr merge-pr; do
      manifest="$ROOT/packages/agents/$harness/.apm/skills/$workflow/SKILL.md"
      if grep -q 'guard-draft.sh' "$manifest"; then return 1; fi
      grep -q 'in this session' "$manifest"
      grep -q 'optional.*write-pr-description\|write-pr-description.*optional' "$manifest"
    done
  done
}
