#!/usr/bin/env bats

@test "devtools grouping rule stays last" {
	run node - "${BATS_TEST_DIRNAME}/../../.github/renovate-config.json5" <<'JS'
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const preset = vm.runInNewContext(`(${fs.readFileSync(process.argv[2], 'utf8')})`);
assert.equal(preset.packageRules.at(-1).groupName, 'devtools');
JS
	[ "$status" -eq 0 ]
}

# Opt in because the ordinary Bats suite does not require Docker or an image pull.
# RUN_RENOVATE_GROUPING=1 mise run test-hooks tests/workflows/renovate-devtools.bats
@test "Renovate groups all thirteen devtools sites and excludes unrelated Go tools" {
	[ "${RUN_RENOVATE_GROUPING:-}" = 1 ] || skip 'set RUN_RENOVATE_GROUPING=1 to run the pinned image'
	bats_require_minimum_version 1.5.0
	local root="${BATS_TEST_DIRNAME}/../.."
	local work="${BATS_TEST_TMPDIR}/fixture"
	local image
	image=$(sed -n 's/.*renovate-image: \([^ ]*\).*/\1/p' "$root/.github/workflows/renovate.yaml")
	[ -n "$image" ]
	# Pull diagnostics must not enter the JSON stream parsed below.
	run docker pull "$image"
	[ "$status" -eq 0 ]
	mkdir -p "$work/consumer/.github/workflows"
	cp "$root/.github/renovate-config.json5" "$work/preset.json5"
	for name in lint test renovate renovate-config-validator; do
		cat >"$work/consumer/.github/workflows/$name.yml" <<YAML
on: push
jobs:
  check:
    uses: tbhb-dev/devtools/.github/workflows/$name.yml@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa # v0.14.1
YAML
	done
	cat >"$work/consumer/.pre-commit-config.yaml" <<'YAML'
repos:
  - repo: https://github.com/tbhb-dev/devtools
    rev: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa # frozen: v0.14.1
    hooks:
      - id: guard-markdown
YAML
	cat >"$work/consumer/vendir.yml" <<'YAML'
apiVersion: vendir.k14s.io/v1alpha1
kind: Config
directories:
  - path: .config/mise/conf.d
    contents:
      - path: .
        git:
          url: https://github.com/tbhb-dev/devtools
          ref: v0.14.1
  - path: .devtools
    contents:
      - path: .
        git:
          url: https://github.com/tbhb-dev/devtools
          ref: v0.14.1
YAML
	printf 'dependencies:\n  apm:\n    - tbhb-dev/devtools#v0.14.1\n' >"$work/consumer/apm.yml"
	cat >"$work/consumer/.github/renovate.json5" <<'JSON5'
{
  extends: [
    "github>tbhb-dev/devtools//.github/renovate-config.json5#aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", // v0.14.1
  ],
}
JSON5
	cat >"$work/consumer/mise.toml" <<'TOML'
[tools]
"go:github.com/tbhb-dev/devtools/cmd/guard-markdown" = "v0.14.1"
"go:github.com/tbhb-dev/devtools/cmd/commit-lint" = "v0.14.1"
"go:github.com/tbhb-dev/devtools/cmd/agentrepo" = "v0.14.1"
"go:github.com/tbhb-dev/devtools/cmd/gh-post" = "v0.14.1"
"go:github.com/fe3dback/go-arch-lint" = "v0.14.1"
"go:github.com/tbhb-dev/other/cmd/tool" = "v0.14.1"
TOML
	cat >"$work/config.cjs" <<'JS'
const fs = require('node:fs');
const JSON5 = require('/usr/local/renovate/node_modules/json5');
module.exports = {
  ...JSON5.parse(fs.readFileSync('../preset.json5', 'utf8')),
  // Extract the pinned preset site without fetching that old preset as policy.
  requireConfig: 'ignored',
  onboarding: false,
  dependencyDashboardApproval: true,
  lockFileMaintenance: { enabled: false },
  githubTokenWarn: false,
  schedule: ['at any time'],
};
JS
	cat >"$work/releases.mjs" <<'JS'
// Managers, rules, version selection, and branch flattening run unchanged.
// Supply remote data and enable final summary logging for the local fixture.
import assert from 'node:assert/strict';
import api from '/usr/local/renovate/dist/modules/datasource/api.js';
import { GlobalConfig } from '/usr/local/renovate/dist/config/global.js';
import { LocalFs } from '/usr/local/renovate/dist/modules/platform/local/scm.js';
import { GithubHttp } from '/usr/local/renovate/dist/util/http/github.js';
// Preset validation reads the fixture's pinned reference even with ignored config.
GithubHttp.prototype.getJsonUnchecked = async url => {
  assert.equal(url, 'repos/tbhb-dev/devtools/contents/.github/renovate-config.json5?ref=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa');
  return { body: { content: Buffer.from('{}').toString('base64') } };
};
// Local mode downgrades full to lookup, which skips branchesInformation.
// Retain full dry-run finalization; dashboard approval prevents file updates.
LocalFs.prototype.branchExists = async () => false;
const set = GlobalConfig.set;
GlobalConfig.set = config => set({ ...config, dryRun: 'full' });
const packages = new Set([
  'tbhb-dev/devtools', 'https://github.com/tbhb-dev/devtools',
  ...['guard-markdown', 'commit-lint', 'agentrepo', 'gh-post'].map(name => `github.com/tbhb-dev/devtools/cmd/${name}`),
  'github.com/fe3dback/go-arch-lint', 'github.com/tbhb-dev/other/cmd/tool',
]);
for (const name of ['github-tags', 'git-tags', 'git-refs', 'go']) {
  const datasource = api.get(name);
  datasource.getReleases = async ({ packageName }) => {
    assert(packages.has(packageName), `Unexpected lookup: ${packageName}`);
    return { releases: ['v0.14.1', 'v0.14.2'].map(version => ({
      version, releaseTimestamp: '2020-01-01T00:00:00Z',
    })) };
  };
  datasource.getDigest = async () => 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb';
}
JS
	tar --mtime=2020-01-01 -C "$work" -cf "$BATS_TEST_TMPDIR/fixture.tar" .
	run --separate-stderr docker run --pull=never --rm -i --network none -w /tmp \
		-e LOG_LEVEL=debug -e LOG_FORMAT=json --entrypoint sh "$image" -c '
      set -eu
      work=$(mktemp -d)
      cd "$work"
      tar -xf -
      export RENOVATE_CONFIG_FILE="$work/config.cjs"
      cd consumer
      node --import "$work/releases.mjs" /usr/local/renovate/dist/renovate.js --platform=local --dry-run=full
    ' <"$BATS_TEST_TMPDIR/fixture.tar"
	printf '%s\n' "$output" >"$BATS_TEST_TMPDIR/renovate.log"
	[ "$status" -eq 0 ]
	run node - "$BATS_TEST_TMPDIR/renovate.log" <<'JS'
const assert = require('node:assert/strict');
const fs = require('node:fs');
const logs = fs.readFileSync(process.argv[2], 'utf8').trim().split('\n').map(line => JSON.parse(line));
assert(!logs.some(log => log.level >= 40 && log.msg !== 'Config needs migrating'), 'Renovate warned or failed');
const summaries = logs.filter(log => log.branchesInformation);
assert.equal(summaries.length, 1);
const branches = summaries[0].branchesInformation;
assert.equal(branches.length, 3);
const grouped = branches.filter(branch => branch.branchName === 'renovate/devtools');
assert.equal(grouped.length, 1);
const upgrades = grouped[0].upgrades;
// Built-in APM and the custom regex manager can report the same APM site.
// vendir has two distinct refs in one file, which must both survive grouping.
assert.equal(upgrades.filter(upgrade => upgrade.packageFile === 'vendir.yml').length, 2);
const sites = [...new Set(upgrades.map(upgrade => `${upgrade.packageFile}:${upgrade.depName}`))].sort();
assert.deepEqual(sites, [
  ...['lint', 'test', 'renovate', 'renovate-config-validator'].map(name => `.github/workflows/${name}.yml:tbhb-dev/devtools`),
  '.pre-commit-config.yaml:tbhb-dev/devtools',
  'vendir.yml:https://github.com/tbhb-dev/devtools',
  'apm.yml:tbhb-dev/devtools', '.github/renovate.json5:tbhb-dev/devtools',
  ...['guard-markdown', 'commit-lint', 'agentrepo', 'gh-post'].map(name => `mise.toml:go:github.com/tbhb-dev/devtools/cmd/${name}`),
].sort());
assert.equal(grouped[0].result, 'needs-approval');
assert(upgrades.every(upgrade => upgrade.newValue === 'v0.14.2'));
for (const tool of ['github.com/fe3dback/go-arch-lint', 'github.com/tbhb-dev/other/cmd/tool']) {
  const control = branches.filter(branch => branch.upgrades.some(upgrade => upgrade.packageName === tool));
  assert.equal(control.length, 1, tool);
  assert.notEqual(control[0].branchName, 'renovate/devtools');
  assert(control[0].upgrades.every(upgrade => upgrade.newValue === 'v0.14.2'));
}
console.log(JSON.stringify(branches, null, 2));
JS
	printf '%s\n' "$output"
	[ "$status" -eq 0 ]
}
