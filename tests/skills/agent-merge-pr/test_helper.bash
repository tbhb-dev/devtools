agent_setup_file() {
  local root
  root=$(cd "$BATS_TEST_DIRNAME/../../.." && pwd)
  (cd "$root" && go build -o "$BATS_FILE_TMPDIR/agentrepo" ./cmd/agentrepo)
  (cd "$root" && go build -o "$BATS_FILE_TMPDIR/commit-lint" ./cmd/commit-lint)
}

# cspell:words noqa
agent_setup() {
  source "${BATS_TEST_DIRNAME}/../merge-pr/test_helper.bash"
  merge_setup
  export PATH="$BATS_FILE_TMPDIR:$PATH"
  export DEVTOOLS_GH_LOGIN=agent DEVTOOLS_REQUIRED_REVIEWER=reviewer
  unset DEVTOOLS_MERGE_OWNER DEVTOOLS_REQUIRED_REVIEWER_TYPE
  AGENT="$ROOT/packages/agents/claude/.apm/skills/agent-merge-pr/scripts"
  POLICY="$SCRIPTS/merge-policy.py"
  git checkout --quiet main
  mkdir -p .agents
  cat >.agents/agent-skills.conf <<'CONF'
lint_squash_cmd = mise run lint-squash-msg
owner_paths = ^owned/
constitutional_paths = ^constitution[.]txt$
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
  export DEVTOOLS_MERGE_OWNER=owner
  pr ".reviews += [{author:{login:\"owner\"},state:\"APPROVED\",commit:{oid:\"$HEAD_OID\"},submittedAt:\"2026-01-02T00:00:00Z\"}]"
}

policy_config() {
  jq -n '{golden_paths:"^golden/",loosening_markers:"noqa|nolint|vale (off|disable)",gate_config_paths:"lint[.]yaml$",owner_paths:"^owned/",disposable_ignored:"^cache/"}'
}

inspect_policy() { policy_config | uv run --quiet --script "$POLICY" inspect "$BASE_OID" "$HEAD_OID"; }
