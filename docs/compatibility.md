# Compatibility policy

devtools stays backward compatible for N-2 releases. Checks in release N must accept a consumer configuration unchanged when release N-2 accepts it. A dependency bump that needs a consumer configuration change is a devtools defect.

New configuration keys are optional. Their defaults keep the previous behavior. Before removing a key or file, devtools must warn consumers for at least two releases. Changing a default requires the same warning period.

Security fixes can break compatibility when necessary. Such a fix uses the transition declaration from [#631](https://github.com/tbhb-dev/devtools/issues/631) to explain the required consumer changes.

## Pull request declarations

[The description validator](../packages/agents/source/skills/pr/shared/scripts/validate-description.sh) defines the paths that require compatibility declarations. When adding a configuration file or directory that consumers use, extend the path list and its tests so later pull requests must declare their compatibility impact.

A pull request touching a listed path must include one of these lines under `## Risk`:

```text
Compatibility: none
Compatibility: deprecates <key> since <version>
```

Use `none` when the change doesn't start a deprecation. For a deprecation, replace `<key>` with the deprecated key and `<version>` with the release that starts its warning period. Consumers can keep their configuration through that period of at least two releases.
