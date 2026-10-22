#!/usr/bin/env bats

@test "the secret scan runs with full history and no persisted credentials" {
	run uv run python - "${BATS_TEST_DIRNAME}/../../.github/workflows/scan-gitleaks.yml" <<'PY'
import re
import sys

import yaml

with open(sys.argv[1], encoding="utf-8") as stream:
    workflow = yaml.safe_load(stream)
job = workflow["jobs"]["gitleaks"]
steps = job["steps"]
assert steps[0]["uses"].split("@")[0] == "step-security/harden-runner"
checkout = next(step for step in steps if step.get("uses", "").split("@")[0] == "actions/checkout")
assert checkout["with"]["fetch-depth"] == 0
assert checkout["with"]["persist-credentials"] is False
assert any(step.get("uses", "").split("@")[0] == "jdx/mise-action" for step in steps)
assert any(step.get("run") == "mise run devtools:gitleaks -- --mode ci" for step in steps)
for step in steps:
    if "uses" in step:
        assert re.search(r"@[0-9a-f]{40}$", step["uses"])
PY
	[ "$status" -eq 0 ]
}

@test "reusable jobs retain scanner failures, SARIF policy and caller contracts" {
	run uv run python - "$BATS_TEST_DIRNAME/../.." <<'PY'
from pathlib import Path
import re
import sys

import yaml

root = Path(sys.argv[1])
workflows = root / ".github/workflows"
for name in ("govulncheck", "gomodscan", "gitleaks"):
    workflow = yaml.safe_load((workflows / f"scan-{name}.yml").read_text())
    assert "workflow_call" in workflow[True]
    job = workflow["jobs"][name]
    for step in job["steps"]:
        assert not step.get("continue-on-error", False)
        if "uses" in step:
            assert re.search(r"@[0-9a-f]{40}$", step["uses"])
    caller = yaml.safe_load((workflows / "security.yml").read_text())
    assert set(caller[True]) == {"push", "pull_request", "schedule"}
    assert caller[True]["push"]["branches"] == ["main"]
    assert caller[True]["schedule"] == [{"cron": "26 6 * * *"}]
    assert caller["jobs"][name]["uses"] == f"./.github/workflows/scan-{name}.yml"
steps = yaml.safe_load((workflows / "scan-govulncheck.yml").read_text())["jobs"]["govulncheck"]["steps"]
artifact = next(s for s in steps if s.get("uses", "").startswith("actions/upload-artifact@"))
assert artifact["with"]["path"] == "govulncheck.sarif"
assert "!cancelled()" in artifact["if"]
upload = steps[-1]
assert upload["with"]["sarif_file"] == "govulncheck-reachable.sarif"
assert upload["with"]["category"] == "govulncheck"
assert steps[-2]["run"] == "bash .devtools/scripts/filter-govulncheck-sarif.sh govulncheck.sarif govulncheck-reachable.sarif"
lint = yaml.safe_load((workflows / "consumer-lint.yml").read_text())["jobs"]["lint"]
assert lint["name"] == "lint"
commands = [step["run"] for step in lint["steps"] if "run" in step]
assert commands == ["mise run devtools:check-vendored", "mise run install-tools", "mise run install-guard-markdown", 'go_path=$(go env GOPATH)\necho "$go_path/bin" >> "$GITHUB_PATH"\n', "mise run lint"]
ci = yaml.safe_load((workflows / "ci.yml").read_text())
assert ci["jobs"]["lint"]["name"] == "lint"
assert ci["jobs"]["test"]["name"].startswith("test (")
PY
	[ "$status" -eq 0 ]
}

@test "Rust advisory workflow uses consumer pins and propagates scan failures" {
	run uv run python - "${BATS_TEST_DIRNAME}/../../.github/workflows/scan-cargo-deny.yml" <<'PY'
import re
import sys

import yaml

with open(sys.argv[1], encoding="utf-8") as stream:
    workflow = yaml.safe_load(stream)
assert "workflow_call" in workflow[True]
assert workflow["permissions"] == {"contents": "read"}
assert workflow["env"]["MISE_LOCKED"] == "1"
steps = workflow["jobs"]["cargo-deny"]["steps"]
assert steps[0]["uses"].startswith("step-security/harden-runner@")
checkout = next(s for s in steps if s.get("uses", "").startswith("actions/checkout@"))
assert checkout["with"]["persist-credentials"] is False
setup = next(s for s in steps if s.get("uses", "").startswith("jdx/mise-action@"))
assert setup["with"]["install_args"] == "rust github:EmbarkStudios/cargo-deny"
assert steps[-1]["run"] == "mise run devtools:vuln-rust"
assert steps[-1]["env"]["MISE_TASK_RUN_AUTO_INSTALL"] == "0"
for step in steps:
    assert not step.get("continue-on-error", False)
    if "uses" in step:
        assert re.search(r"@[0-9a-f]{40}$", step["uses"])
PY
	echo "$output"
	[ "$status" -eq 0 ]
}
