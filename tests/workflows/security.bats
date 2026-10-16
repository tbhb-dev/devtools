#!/usr/bin/env bats

@test "the secret scan runs with full history and no persisted credentials" {
  run uv run python - "${BATS_TEST_DIRNAME}/../../.github/workflows/security.yml" <<'PY'
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
assert any(step.get("run") == "mise run devtools:gitleaks" for step in steps)
for step in steps:
    if "uses" in step:
        assert re.search(r"@[0-9a-f]{40}$", step["uses"])
PY
  [ "$status" -eq 0 ]
}
