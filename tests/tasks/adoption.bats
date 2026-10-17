#!/usr/bin/env bats
# A clean consumer extends the shipped interfaces without copying helpers.

setup() {
  REPO="${BATS_TEST_DIRNAME}/../.."
  WORK="${BATS_TEST_TMPDIR}/consumer"
  mkdir -p "$WORK/.config/mise/conf.d" "$WORK/.devtools"
  cp -R "$REPO/.devtools/." "$WORK/.devtools/"
  cp "$REPO/.config/mise/conf.d/"{10-devtools-tools,20-devtools-env,30-devtools-skill-contract}.toml "$WORK/.config/mise/conf.d/"
  export MISE_TRUSTED_CONFIG_PATHS="$WORK"
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
  unset GIT_DIR GIT_COMMON_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY
  git init -q "$WORK"
  cat > "$WORK/mise.toml" <<'TOML'
[tools]
uv = "0.12.23"
[task_config]
includes = [".devtools/tasks/apm.toml"]
[tasks.lint-draft]
extends = "devtools:lint-draft"
[tasks.fix-prose-replacements]
extends = "devtools:fix-prose-replacements"
TOML
  # Use the installed Vale package and its published output template.
  cp -R "$REPO/.vale" "$WORK/.vale"
  cat > "$WORK/.vale.ini" <<'INI'
StylesPath = .vale
MinAlertLevel = warning
[*.md]
BasedOnStyles = Google, ai-tells
Google.Contractions = error
INI
  printf '{"language":"en"}\n' > "$WORK/.cspell.jsonc"
  cd "$WORK"
}

@test "payload defaults lint and replace prose through the real consumer tasks" {
  printf '# Draft\n\nYou do not need this file.\n' > draft.md
  run mise run lint-draft draft.md
  echo "$output"
  [ "$status" -ne 0 ]
  [[ "$output" == *'replace_with='* ]]
  run mise run fix-prose-replacements draft.md
  echo "$output"
  [ "$status" -eq 0 ]
  grep -Fq "You don't need this file." draft.md
  run mise run lint-draft draft.md
  echo "$output"
  [ "$status" -eq 0 ]
}

@test "APM reconciliation preserves other settings and removes obsolete hooks" {
  mkdir -p .claude .agents .codex bin
  printf '{"hooks":{"obsolete":[]},"permissions":{"allow":["Read"]}}\n' > .claude/settings.json
  printf '{"apm":{"obsolete":[]},"local":true}\n' > .agents/hooks.json
  printf '{"hooks":{"obsolete":[]},"local":true}\n' > .codex/hooks.json
  touch .claude/apm-hooks.json .agents/apm-hooks.json .codex/apm-hooks.json
  cat > bin/apm <<'SH'
#!/usr/bin/env bash
set -eu
if [ "$1" = install ]; then
  jq -e '.permissions.allow == ["Read"] and (has("hooks") | not)' .claude/settings.json
  jq -e '.local and (has("apm") | not)' .agents/hooks.json
  jq -e '.local and (has("hooks") | not)' .codex/hooks.json
  test ! -f .claude/apm-hooks.json
  test ! -f .agents/apm-hooks.json
  test ! -f .codex/apm-hooks.json
  touch installed
else
  test -f installed
  printf '{"checks":[{"name":"deployment","passed":true,"message":"clean","details":[]}]}\n'
fi
SH
  chmod +x bin/apm
  run env PATH="$WORK/bin:$PATH" mise run devtools:apm-sync
  echo "$output"
  [ "$status" -eq 0 ]
  [[ "$output" == *'APM audit passed'* ]]
}

@test "shipped helpers match the publisher implementations" {
  for helper in lint-draft.sh fix_prose_replacements.py apm_audit.py; do
    cmp "$REPO/tools/$helper" ".devtools/scripts/$helper"
  done
}

@test "real APM deploy and audit reconcile a local package in a clean consumer" {
  mkdir -p package/.apm/hooks .claude
  cat > package/apm.yml <<'YAML'
name: adoption-fixture
version: 1.0.0
description: Adoption runtime fixture
targets:
  - claude
YAML
  cat > apm.yml <<'YAML'
name: consumer
version: 1.0.0
description: Adoption runtime acceptance
targets:
  - claude
dependencies:
  apm:
    - ./package
YAML
  cp "$REPO/packages/agents/claude/.apm/hooks/guard-markdown.json" package/.apm/hooks/current.json
  printf '{"hooks":{"Stop":[]},"permissions":{"allow":["Read"]}}\n' > .claude/settings.json
  sed -i '/uv = /a apm = "0.33.0"' mise.toml
  run mise run devtools:apm-sync
  echo "$output"
  [ "$status" -eq 0 ]
  jq -e '.permissions.allow == ["Read"] and (.hooks | has("Stop") | not)' .claude/settings.json
  test -f apm.lock.yaml
}
