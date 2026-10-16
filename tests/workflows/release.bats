#!/usr/bin/env bats

@test "release installs APM before the lockfile refresh in cog's commit" {
  run uv run python - "${BATS_TEST_DIRNAME}/../.." <<'PY'
import sys
import tomllib
from pathlib import Path

import yaml

root = Path(sys.argv[1])
workflow = yaml.safe_load((root / ".github/workflows/release.yml").read_text())
steps = workflow["jobs"]["release"]["steps"]
setup = next(i for i, step in enumerate(steps) if step.get("with", {}).get("install_args") == "apm")
bump = next(i for i, step in enumerate(steps) if step.get("with", {}).get("command") == "bump")
replay = next(step for step in steps if step.get("id") == "commit")
assert steps[setup]["uses"].startswith("jdx/mise-action@")
assert setup < bump < steps.index(replay)
hooks = tomllib.loads((root / "cog.toml").read_text())["pre_bump_hooks"]
assert hooks[-1] == "apm install"
assert "git diff -z --name-only --diff-filter=d HEAD~1 HEAD" in replay["run"]
assert "git rev-parse 'HEAD^{tree}'" in replay["run"]
assert 'git rev-parse "$oid^{tree}"' in replay["run"]
PY
  [ "$status" -eq 0 ]
}
