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

setup() {
	source "${BATS_TEST_DIRNAME}/../test_helper.bash"
	isolate_git
	ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
	mkdir -p "$BATS_TEST_TMPDIR/bin"
}

extract_step() {
	uv run python - "$ROOT/.github/workflows/$1.yml" "$2" >"$BATS_TEST_TMPDIR/step.sh" <<'PY'
import sys
import yaml
from pathlib import Path
workflow = yaml.safe_load(Path(sys.argv[1]).read_text())
steps = next(iter(workflow["jobs"].values()))["steps"]
print(next(s["run"] for s in steps if s.get("id", s.get("name")) == sys.argv[2]))
PY
}

@test "release replay stops before publishing when metadata prints output then fails" {
	extract_step release commit
	cat >"$BATS_TEST_TMPDIR/bin/git" <<'SH'
#!/bin/bash
case "$*" in
  'tag --points-at HEAD') echo v1.0.0 ;;
  'diff -z --name-only --diff-filter=d HEAD~1 HEAD') printf 'file\0' ;;
  'rev-parse HEAD~1') echo parent ;;
  'log -1 --format=%s') echo headline ;;
  'log -1 --format=%b') echo body ;;
  'rev-parse '*tree*) echo tree ;;
  'reset '*) touch reset ;;
esac
if [[ $* == "$FAIL_SPEC" ]]; then exit 23; fi
SH
	cat >"$BATS_TEST_TMPDIR/bin/jq" <<'SH'
#!/bin/bash
if [[ $1 == -s ]]; then echo 1; else cat >/dev/null; echo '{}'; fi
SH
	printf '#!/bin/bash\ntouch published; echo oid\n' >"$BATS_TEST_TMPDIR/bin/gh"
	chmod +x "$BATS_TEST_TMPDIR/bin/"*
	cd "$BATS_TEST_TMPDIR"
	touch file
	export PATH="$BATS_TEST_TMPDIR/bin:$PATH" RUNNER_TEMP="$BATS_TEST_TMPDIR"
	export REPO=example/repo BRANCH=main
	for FAIL_SPEC in 'diff -z --name-only --diff-filter=d HEAD~1 HEAD' 'rev-parse HEAD~1' 'log -1 --format=%s' 'log -1 --format=%b'; do
		export FAIL_SPEC
		run bash -e step.sh
		[ "$status" -eq 23 ]
		[ ! -e published ]
	done
}

@test "release replay stops before resetting when both tree probes fail identically" {
	extract_step release commit
	# Reach the tree comparison without a real API call or Git repository.
	sed -n '/^git fetch --quiet origin/,$p' "$BATS_TEST_TMPDIR/step.sh" >"$BATS_TEST_TMPDIR/trees.sh"
	cat >"$BATS_TEST_TMPDIR/bin/git" <<'SH'
#!/bin/bash
case "$1" in
  rev-parse) echo usable; exit 23 ;;
  reset) touch reset ;;
esac
SH
	chmod +x "$BATS_TEST_TMPDIR/bin/git"
	cd "$BATS_TEST_TMPDIR"
	export PATH="$BATS_TEST_TMPDIR/bin:$PATH" BRANCH=main oid=commit
	run bash -e trees.sh
	[ "$status" -eq 23 ]
	[ ! -e reset ]
}

@test "consumer PATH setup preserves a failing go env status" {
	extract_step consumer-lint 'Add Go binaries to PATH'
	printf '#!/bin/bash\necho usable; exit 23\n' >"$BATS_TEST_TMPDIR/bin/go"
	chmod +x "$BATS_TEST_TMPDIR/bin/go"
	export PATH="$BATS_TEST_TMPDIR/bin:$PATH" GITHUB_PATH="$BATS_TEST_TMPDIR/github-path"
	run bash -e "$BATS_TEST_TMPDIR/step.sh"
	[ "$status" -eq 23 ]
	[ ! -e "$GITHUB_PATH" ]
}
