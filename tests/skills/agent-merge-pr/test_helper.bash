# cspell:words noqa hashlib hexdigest rglob
agent_setup() {
  source "$(dirname "${BASH_SOURCE[0]}")/../merge-pr/test_helper.bash"
  merge_setup
  if [ ! -x "$BATS_SUITE_TMPDIR/agentrepo" ] || [ ! -x "$BATS_SUITE_TMPDIR/commit-lint" ]; then
    (cd "$ROOT" && go build -o "$BATS_SUITE_TMPDIR/agentrepo" ./cmd/agentrepo)
    (cd "$ROOT" && go build -o "$BATS_SUITE_TMPDIR/commit-lint" ./cmd/commit-lint)
  fi
  export PATH="$BATS_SUITE_TMPDIR:$PATH"
  machine gh_login agent
  machine required_reviewer reviewer
  unset DEVTOOLS_MERGE_OWNER DEVTOOLS_REQUIRED_REVIEWER_TYPE
  mkdir -p .claude/skills
  cp -R "$ROOT/packages/agents/claude/.apm/skills/merge-pr" .claude/skills/
  cp -R "$ROOT/packages/agents/claude/.apm/skills/agent-merge-pr" .claude/skills/
  cp -R "$ROOT/packages/agents/claude/.apm/skills/review-squash-message" .claude/skills/
  SCRIPTS="$REPO/.claude/skills/merge-pr/scripts"
  AGENT="$REPO/.claude/skills/agent-merge-pr/scripts"
  # Local packages pin skill contents to base blobs instead of lock hashes.
  cat >apm.lock.yaml <<'LOCK'
dependencies:
- repo_url: _local/claude
  source: local
  local_path: ./packages/agents/claude
  deployed_files:
  - .claude/skills/merge-pr
  - .claude/skills/agent-merge-pr
LOCK
  git add -- .claude/skills/merge-pr .claude/skills/agent-merge-pr .claude/skills/review-squash-message apm.lock.yaml
  POLICY="$SCRIPTS/merge-policy.py"
  git checkout --quiet main
  mkdir -p .agents
  cat >.agents/agent-skills.conf <<'CONF'
lint_squash_cmd = mise run lint-squash-msg
owner_paths = ^owned/
golden_paths = ^golden/
loosening_markers = noqa|nolint|shellcheck disable|vale (off|disable)
gate_config_paths = (^|/)(lint|hooks)\.(ya?ml|toml)$
disposable_ignored = ^cache/
CONF
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: configure policy'
  BASE_OID=$(git rev-parse HEAD)
  git checkout --quiet topic
  git rebase --quiet main
  HEAD_OID=$(git rev-parse HEAD)
  git remote add origin "$GH_FIXTURES/upstream.git"
  git push --quiet --force-with-lease="refs/heads/topic:$(git -C "$GH_FIXTURES/upstream.git" rev-parse topic)" origin main topic
  base_at "$BASE_OID"
  pr ".headRefOid=\"$HEAD_OID\" | .labels=[] | .reviews=[{author:{login:\"reviewer\"},state:\"APPROVED\",commit:{oid:\"$HEAD_OID\"},submittedAt:\"2026-01-01T00:00:00Z\"}]"
  echo '[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"test"}]}}]' >"$GH_FIXTURES/api/repos_{owner}_{repo}_rules_branches_main.json"
  export REAL_MERGE=1
}

change() {
  mkdir -p "$(dirname "$1")"
  printf '%s\n' "$2" >"$1"
  git add -- "$1"
  git commit --quiet -m 'test: change policy input'
  HEAD_OID=$(git rev-parse HEAD)
  git push --quiet --force-with-lease="refs/heads/topic:$(git -C "$GH_FIXTURES/upstream.git" rev-parse topic)" origin topic
  pr ".headRefOid=\"$HEAD_OID\" | .reviews[0].commit.oid=\"$HEAD_OID\""
}

approve_owner() {
  machine merge_owner owner
  pr ".reviews += [{author:{login:\"owner\"},state:\"APPROVED\",commit:{oid:\"$HEAD_OID\"},submittedAt:\"2026-01-02T00:00:00Z\"}]"
}

policy_config() {
  jq -n '{golden_paths:"^golden/",loosening_markers:"noqa|nolint|vale (off|disable)",gate_config_paths:"lint[.]yaml$",owner_paths:"^owned/",disposable_ignored:"^cache/"}'
}

inspect_policy() { policy_config | uv run --quiet --script "$POLICY" inspect "$BASE_OID" "$HEAD_OID"; }

# Machine authority is outside the test checkout and never an environment override.
machine() {
  local file=$XDG_CONFIG_HOME/devtools/devtools.conf
  mkdir -p "$(dirname "$file")"
  touch "$file"
  sed "/^$1 =/d" "$file" >"$file.new"
  mv "$file.new" "$file"
  printf '%s = %s\n' "$1" "${2:-}" >>"$file"
}

# Build the remote-package lock metadata from the same deployed directory fixture.
remote_lock() {
  python3 - <<'LOCK'
import hashlib
from pathlib import Path
import json
files = sorted(p for name in ("merge-pr", "agent-merge-pr") for p in Path(".claude/skills", name).rglob("*") if p.is_file())
text = "dependencies:\n- repo_url: o/skills\n  deployed_files:\n"
text += "".join("  - " + str(p) + "\n" for p in files)
text += "  deployed_file_hashes:\n"
text += "".join("    " + json.dumps(str(p)) + ": sha256:" + hashlib.sha256(p.read_bytes()).hexdigest() + "\n" for p in files)
Path("apm.lock.yaml").write_text(text)
LOCK
}

# Publish fixture base configuration before judging the topic against it.
pin_base_config() {
  git add -- .agents/agent-skills.conf
  git commit --quiet -m 'test: pinned base policy'
  BASE_OID=$(git rev-parse HEAD)
  base_at "$BASE_OID"
  git checkout --quiet topic
  git merge --quiet --no-edit main
  HEAD_OID=$(git rev-parse HEAD)
  pr ".headRefOid=\"$HEAD_OID\" | .reviews[0].commit.oid=\"$HEAD_OID\""
  git push --quiet origin main topic
}
