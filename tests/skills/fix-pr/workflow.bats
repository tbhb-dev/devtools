#!/usr/bin/env bats

setup() {
  ROOT="$(cd "${BATS_TEST_DIRNAME}/../../.." && pwd)"
}

@test "unattended description refresh routes through agent-pr (#294)" {
  for harness in claude codex; do
    skill="$ROOT/packages/agents/source/skills/fix-pr/SKILL.${harness}.md"
    grep -Fq "agent-pr/scripts/check.sh" "$skill"
    grep -Fq "agent-pr/scripts/review.sh" "$skill"
    grep -Fq "agent-pr/scripts/publish.sh" "$skill"
    grep -Fq "never pass \`--confirmed\`" "$skill"
  done
}

@test "interactive description refresh keeps the pr route (#294)" {
  for harness in claude codex; do
    skill="$ROOT/packages/agents/source/skills/fix-pr/SKILL.${harness}.md"
    grep -Fq "pr/scripts/review.sh" "$skill"
    grep -Fq "pr/scripts/create-pr.sh" "$skill"
  done
}

@test "unattended amend sends rewrite through agent-pr (#294)" {
  for harness in claude codex; do
    skill="$ROOT/packages/agents/source/skills/fix-pr/SKILL.${harness}.md"
    grep -Eq 'agent-pr/scripts/publish\.sh.*--rewrite' "$skill"
  done
}

@test "headless history choice defaults to a separate commit (#295)" {
  for harness in claude codex; do
    skill="$ROOT/packages/agents/source/skills/fix-pr/SKILL.${harness}.md"
    grep -Fq "In a headless session with no supplied strategy, use a separate commit without prompting." "$skill"
    grep -Fq "No headless default authorizes a force-push." "$skill"
  done
}

@test "explicit history choices and interactive prompts remain available (#295)" {
  grep -Fq 'ask before committing with `AskUserQuestion`' \
    "$ROOT/packages/agents/source/skills/fix-pr/SKILL.claude.md"
  grep -Fq 'in Default mode ask in the final response' \
    "$ROOT/packages/agents/source/skills/fix-pr/SKILL.codex.md"
  for harness in claude codex; do
    skill="$ROOT/packages/agents/source/skills/fix-pr/SKILL.${harness}.md"
    grep -Fq "An explicit caller choice can still select an amend and force-with-lease." "$skill"
  done
}
