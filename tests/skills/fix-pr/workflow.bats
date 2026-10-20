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
