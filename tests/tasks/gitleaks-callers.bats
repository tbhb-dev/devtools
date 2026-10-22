#!/usr/bin/env bats

@test "AC7 AC9 callers select explicit modes and generated unattended delegates share gates" {
  run uv run python - "$BATS_TEST_DIRNAME/../.." <<'PY'
from pathlib import Path
import sys
import tomllib

root = Path(sys.argv[1])
tasks = tomllib.loads((root / "mise.toml").read_text())["tasks"]
for name in ("security", "check-all"):
    assert "mise run devtools:gitleaks -- --mode ci" in tasks[name]["run"]
source = root / "packages/agents/source/skills"
commit = (source / "commit/shared/scripts/commit.sh").read_text()
assert "mise run devtools:gitleaks -- --mode local\n" in commit
pr = (source / "pr/shared/scripts/create-pr.sh").read_text()
assert 'scan_base="refs/remotes/origin/$base"' in pr
assert 'reviewed_base_oid=$(dt_stamp_value pr base_oid)' in pr
assert '[ "$scan_base_oid" = "$reviewed_base_oid" ] || die' in pr
assert 'mise run devtools:gitleaks -- --mode local --base "$scan_base"\nrequire_reviewed' in pr
assert "mise run devtools:gitleaks -- --mode ci" in (source / "fix-pr/shared/scripts/diagnose.sh").read_text()
for harness in ("claude", "codex"):
    generated = root / f"packages/agents/{harness}/.apm/skills"
    assert (generated / "commit/scripts/commit.sh").read_text() == commit
    assert (generated / "pr/scripts/create-pr.sh").read_text() == pr
    assert 'commit/scripts/commit.sh"' in (generated / "agent-commit/scripts/commit.sh").read_text()
    assert 'pr/scripts/create-pr.sh"' in (generated / "agent-pr/scripts/publish.sh").read_text()
for script in (source / "merge-pr").glob("**/*.sh"):
    assert "mise run devtools:gitleaks" not in script.read_text()
docs = (root / "docs/gitleaks.md").read_text()
for phrase in ("full index", "no local default", "reduces local security coverage", "human review", "current head"):
    assert phrase in docs
assert "--mode ci" in (root / "docs/consumer-workflows.md").read_text()
PY
  echo "$output"
  [ "$status" -eq 0 ]
}
