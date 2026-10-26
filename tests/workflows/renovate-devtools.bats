#!/usr/bin/env bats

@test "devtools grouping rule stays last" {
	run node - "${BATS_TEST_DIRNAME}/../../.github/renovate-config.json5" <<'JS'
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const preset = vm.runInNewContext(`(${fs.readFileSync(process.argv[2], 'utf8')})`);
assert.equal(preset.packageRules.at(-1).groupName, 'devtools');
assert.equal(preset.packageRules.at(-1).minimumReleaseAge, null);
assert.deepEqual(Array.from(preset.packageRules.at(-1).schedule), ['at any time']);
JS
	[ "$status" -eq 0 ]
}

# Opt in because the ordinary Bats suite does not require Docker or an image pull.
# RUN_RENOVATE_GROUPING=1 mise run test-hooks tests/workflows/renovate-devtools.bats
@test "Renovate updates devtools immediately and retains third-party timing gates" {
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
  dependencyDashboardApproval: false,
  lockFileMaintenance: { enabled: false },
  githubTokenWarn: false,
  gitAuthor: 'Fixture <fixture@example.com>',
  // Exercise pin edits offline without fetching vendored payloads or tools.
  skipArtifactsUpdate: true,
};
JS
	cat >"$work/releases.mjs" <<'JS'
// Managers, rules, version selection, and branch flattening run unchanged.
// Supply remote data and enable final summary logging for the local fixture.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { registerHooks } from 'node:module';
// Local platform omits Git reads needed by the write path. Redirect that I/O
// boundary to the consumer tree; manager replacement logic remains unchanged.
registerHooks({ load(url, context, nextLoad) {
  const result = nextLoad(url, context);
  if (url.endsWith('/util/git/index.js')) {
    return { ...result, source: String(result.source).replace(
      'async function getFile(filePath, branchName) {',
      `async function getFile(filePath, branchName) {
        return (await import('node:fs/promises')).readFile(filePath, 'utf8').catch(() => null);
      }
async function unusedRemoteGetFile(filePath, branchName) {`) };
  }
  return result;
} });
const { default: platformApi } = await import('/usr/local/renovate/dist/modules/platform/api.js');
const now = process.env.FIXTURE_NOW;
const RealDate = Date;
globalThis.Date = class extends RealDate {
  constructor(...args) { super(...(args.length ? args : [now])); }
  static now() { return new RealDate(now).getTime(); }
};
const statePath = '../state.json';
const state = fs.existsSync(statePath)
  ? JSON.parse(fs.readFileSync(statePath, 'utf8')) : { branches: [], prs: [] };
const save = () => fs.writeFileSync(statePath, JSON.stringify(state));
const local = platformApi.get('local');
platformApi.set('local', {
  ...local,
  getPrList: async () => state.prs,
  findPr: async () => null,
  getBranchPr: async branch => state.prs.find(pr => pr.sourceBranch === branch) ?? null,
  getPr: async number => state.prs.find(pr => pr.number === number) ?? null,
  createPr: async config => {
    assert(!state.prs.some(pr => pr.sourceBranch === config.sourceBranch), 'duplicate PR intent');
    const pr = { ...config, number: state.prs.length + 1, state: 'open', labels: [],
      title: config.prTitle, body: config.prBody };
    state.prs.push(pr); save(); return pr;
  },
});
const { default: api } = await import('/usr/local/renovate/dist/modules/datasource/api.js');
const { GlobalConfig } = await import('/usr/local/renovate/dist/config/global.js');
const { LocalFs } = await import('/usr/local/renovate/dist/modules/platform/local/scm.js');
const { GithubHttp } = await import('/usr/local/renovate/dist/util/http/github.js');
// Preset validation reads the fixture's pinned reference even with ignored config.
GithubHttp.prototype.getJsonUnchecked = async url => {
  assert.match(url, /^repos\/tbhb-dev\/devtools\/contents\/\.github\/renovate-config\.json5\?ref=(a{40}|b{40})$/);
  return { body: { content: Buffer.from('{}').toString('base64') } };
};
// Local mode downgrades full to lookup, which skips branchesInformation.
// Retain remote branch and PR state while Renovate updates real fixture files.
LocalFs.prototype.branchExists = async branch => state.branches.includes(branch);
LocalFs.prototype.getBranchCommit = async () => 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb';
LocalFs.prototype.commitAndPush = async config => {
  assert.equal(config.branchName, 'renovate/devtools');
  for (const file of config.files) {
    assert.equal(file.type, 'addition');
    const dest = path.join('../branch-tree', file.path);
    fs.mkdirSync(path.dirname(dest), { recursive: true });
    fs.writeFileSync(dest, file.contents);
  }
  if (!state.branches.includes(config.branchName)) state.branches.push(config.branchName);
  save();
  return 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb';
};
const set = GlobalConfig.set;
GlobalConfig.set = config => set({ ...config, dryRun: null });
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
      version, releaseTimestamp: version === 'v0.14.1' || process.env.FIXTURE_AGE === 'old'
        ? '2020-01-01T00:00:00Z' : new Date(Date.now() - 3600000).toISOString(),
    })) };
  };
  datasource.getDigest = async () => 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb';
}
JS
	# Run the consumer's actual pin-consistency task against Renovate's output.
	python3 - "$root/.devtools/tasks/pins.toml" "$work/check-pins.sh" <<'PYTHON'
import pathlib
import sys
import tomllib
config = tomllib.loads(pathlib.Path(sys.argv[1]).read_text())
pathlib.Path(sys.argv[2]).write_text(config['devtools:check-pins']['run'])
PYTHON
	tar --mtime=2020-01-01 -C "$work" -cf "$BATS_TEST_TMPDIR/fixture.tar" .
	run --separate-stderr docker run --pull=never --rm -i --network none -w /tmp \
		-e LOG_LEVEL=debug -e LOG_FORMAT=json --entrypoint sh "$image" -c '
      set -eu
      work=$(mktemp -d)
      cd "$work"
      tar -xf -
      export RENOVATE_CONFIG_FILE="$work/config.cjs"
      cd consumer
      cp -R . ../base-consumer
      export FIXTURE_NOW=2026-10-26T12:00:00Z FIXTURE_AGE=young
      cp -R ../base-consumer/. .
      node --import "$work/releases.mjs" /usr/local/renovate/dist/renovate.js --platform=local
      cp -R ../base-consumer ../updated-consumer
      cp -R ../branch-tree/. ../updated-consumer/
      (cd ../updated-consumer && git init -q && git add -- .github .pre-commit-config.yaml apm.yml vendir.yml mise.toml && bash ../check-pins.sh)
      cp -R ../base-consumer/. .
      node --import "$work/releases.mjs" /usr/local/renovate/dist/renovate.js --platform=local
      cp ../state.json ../replay-state.json
      rm ../state.json
      export FIXTURE_NOW=2026-10-25T12:00:00Z
      cp -R ../base-consumer/. .
      node --import "$work/releases.mjs" /usr/local/renovate/dist/renovate.js --platform=local
      rm ../state.json
      export FIXTURE_NOW=2026-10-26T12:00:00Z FIXTURE_AGE=old
      cp -R ../base-consumer/. .
      node --import "$work/releases.mjs" /usr/local/renovate/dist/renovate.js --platform=local
      cat ../replay-state.json
    ' <"$BATS_TEST_TMPDIR/fixture.tar"
	printf '%s\n' "$output" >"$BATS_TEST_TMPDIR/renovate.log"
	if [ "$status" -ne 0 ]; then
		printf '%s\n%s\n' "$output" "$stderr"
		return 1
	fi
	run node - "$BATS_TEST_TMPDIR/renovate.log" <<'JS'
const assert = require('node:assert/strict');
const fs = require('node:fs');
const output = fs.readFileSync(process.argv[2], 'utf8');
assert(output.includes('every devtools pin is on v0.14.2'));
const logs = output.trim().split('\n').filter(line => line.startsWith('{')).map(line => JSON.parse(line));
assert.deepEqual(logs.filter(log => log.level >= 40 && log.msg !== 'Config needs migrating'), [], 'Renovate warned or failed');
const summaries = logs.filter(log => log.branchesInformation);
assert.equal(summaries.length, 4);
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
assert.equal(grouped[0].result, 'pr-created');
assert(upgrades.every(upgrade => upgrade.newValue === 'v0.14.2'));
for (const tool of ['github.com/fe3dback/go-arch-lint', 'github.com/tbhb-dev/other/cmd/tool']) {
  const control = branches.filter(branch => branch.upgrades.some(upgrade => upgrade.packageName === tool));
  assert.equal(control.length, 1, tool);
  assert.notEqual(control[0].branchName, 'renovate/devtools');
  assert(control[0].upgrades.every(upgrade => upgrade.newValue === 'v0.14.2'));
  for (const [index, result] of [[2, 'pending'], [3, 'not-scheduled']]) {
    const held = summaries[index].branchesInformation.find(branch =>
      branch.upgrades.some(upgrade => upgrade.packageName === tool));
    assert.equal(held.result, result, `${tool}: ${result}`);
  }
}
for (const summary of summaries) {
  const devtools = summary.branchesInformation.filter(branch =>
    branch.upgrades.some(upgrade => upgrade.packageName.includes('tbhb-dev/devtools')));
  assert.equal(devtools.length, 1);
  assert.equal(devtools[0].branchName, 'renovate/devtools');
}
assert.equal(summaries[1].branchesInformation.find(branch => branch.branchName === 'renovate/devtools').result, 'done');
const state = logs.find(log => log.branches && log.prs);
assert.deepEqual(state.branches, ['renovate/devtools']);
assert.equal(state.prs.length, 1);
assert.equal(state.prs[0].sourceBranch, 'renovate/devtools');
console.log('AC1–AC5: timing, grouping, applied pins, and retained-state replay passed');
JS
	printf '%s\n' "$output"
	[ "$status" -eq 0 ]
}
