#!/usr/bin/env bats

@test "Rust MSRV preset matches Cargo and Clippy versions with the mise Rust identity" {
  run node - "${BATS_TEST_DIRNAME}/../.." <<'JS'
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const root = process.argv[2];
const preset = JSON.parse(fs.readFileSync(path.join(root, '.github/renovate-rust-msrv-config.json5'), 'utf8'));
const fixture = path.join(root, 'tests/workflows/fixtures/rust-msrv');
const mise = fs.readFileSync(path.join(fixture, 'mise.toml'), 'utf8');
const currentValue = /rust = \{ version = "([^"]+)"/.exec(mise)[1];
assert.equal(currentValue, '1.97.1');
assert.deepEqual(preset.packageRules, [{
  matchDatasources: ['rust-version'], matchPackageNames: ['rust'], groupName: 'rust-toolchain',
}]);
function update(file, content) {
  const dependencies = [];
  for (const manager of preset.customManagers) {
    if (!manager.managerFilePatterns.some(pattern => new RegExp(pattern.slice(1, -1)).test(file))) continue;
    assert.equal(manager.depNameTemplate, 'rust');
    assert.equal(manager.datasourceTemplate, 'rust-version');
    assert.equal(manager.versioningTemplate, 'semver');
    for (const pattern of manager.matchStrings) {
      content = content.replace(new RegExp(pattern, 'g'), (...args) => {
        const groups = args.at(-1);
        dependencies.push(groups.currentValue);
        return args[0].replace(groups.currentValue, '1.99.0');
      });
    }
  }
  return { content, dependencies };
}
for (const file of ['Cargo.toml', 'member/Cargo.toml', 'clippy.toml']) {
  const before = fs.readFileSync(path.join(fixture, file), 'utf8');
  const result = update(file, before);
  assert.deepEqual(result.dependencies, [currentValue], file);
  assert(result.content.includes('1.99.0'), file);
  if (file.endsWith('Cargo.toml')) assert(result.content.includes('rust-version = "1.70.0"'), file);
}
for (const file of ['.clippy.toml', 'member/clippy.toml', 'member/.clippy.toml']) {
  assert.deepEqual(update(file, "# policy\r\nmsrv = '1.97.1'\r\n").dependencies, [currentValue]);
}
for (const content of [
  '[package]\nrust-version.workspace = true\n',
  '[package]\nname = "member"\n[package.metadata]\nrust-version = "1.70.0"\n',
  '[workspace.package]\nedition = "2024"\n[workspace.metadata]\nrust-version = "1.70.0"\n',
  '[package]\n# rust-version = "1.70.0"\n',
]) assert.deepEqual(update('Cargo.toml', content).dependencies, []);
assert.deepEqual(update('other.toml', 'msrv = "1.97.1"\n').dependencies, []);
const general = fs.readFileSync(path.join(root, '.github/renovate-config.json5'), 'utf8');
assert(!general.includes('"github>tbhb-dev/devtools//.github/renovate-rust-msrv-config.json5"'));
JS
  [ "$status" -eq 0 ]
}

@test "Renovate validator covers the opt-in Rust preset" {
  run python3 - "${BATS_TEST_DIRNAME}/../../.github/workflows/renovate-config-validator.yaml" <<'PY'
import pathlib
import sys

workflow = pathlib.Path(sys.argv[1]).read_text().replace('\\\n', '')
assert '"**renovate-rust-msrv-config.json5"' in workflow
loops = [line for line in workflow.split('\n') if 'for preset in ' in line]
assert len(loops) == 2
assert all('.github/renovate-rust-msrv-config.json5' in line for line in loops)
PY
  [ "$status" -eq 0 ]
}
