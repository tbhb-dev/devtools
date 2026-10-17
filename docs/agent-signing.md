# Agent commit signing

`ssh-sign` is the `gpg.ssh.program` entry point. It forwards Git's arguments to OpenSSH with a dedicated agent socket. Other SSH commands keep the session's `SSH_AUTH_SOCK`. It works without space-geese installed.

## Install after release

The operator runs `mise run devtools:signing-agent-install` from a devtools checkout. It builds the command into `~/.local/bin/ssh-sign` and renders `launchd` and `systemd` user templates into `~/.local/share/devtools/signing/`. Installation doesn't start services, edit Git configuration, or change agent identities. OpenSSH must be on PATH. The 1Password provider also needs `op`.

Create `~/.config/devtools/signing.env` with mode 600. This file contains data rather than a shell script. Use literal `DEVTOOLS_SIGNING_NAME=value` lines without quotes, variable expansion, or `export`. Environment values override the file. `DEVTOOLS_SIGNING_CONFIG` selects another file. Paths must be absolute.

```dotenv
DEVTOOLS_SIGNING_PROVIDER=op
DEVTOOLS_SIGNING_PUBLIC_KEY=/home/operator/.config/devtools/signing-key.pub
DEVTOOLS_SIGNING_OP_REFERENCE=REPLACE_WITH_PRIVATE_KEY_REFERENCE
DEVTOOLS_SIGNING_KEY_TTL=3600
DEVTOOLS_SIGNING_RELOAD=2700
```

Set `DEVTOOLS_SIGNING_OP_REFERENCE` to the private-key field reference copied from 1Password, with `?ssh-format=openssh` appended. Copy the public signing key to the configured public-key path. For 1Password, provide authentication through the existing `op` environment, or set `DEVTOOLS_SIGNING_TOKEN_COMMAND` to an executable that prints the service-account token. The command takes no arguments. A small operator-owned executable can call macOS Keychain with the required arguments. The wrapper passes the token only to `op`. Key output flows directly from `op read` into `ssh-add -t 3600 -`, with no private-key file or command argument.

For a host with an existing key file, such as a Linux host, replace the provider settings:

```dotenv
DEVTOOLS_SIGNING_PROVIDER=file
DEVTOOLS_SIGNING_KEY_FILE=/home/operator/.ssh/agent-signing-key
```

Use a protected key file that doesn't require interaction. This provider loads the existing file into the dedicated agent with the same lifetime. It doesn't create a copy.

`DEVTOOLS_SIGNING_RUN_DIR` defaults to `~/.local/state/devtools/signing`. It contains `agent.sock`, a `key-loaded-at` timestamp, and a transient lock directory. The wrapper serializes startup and renewal across signers. Each call probes with a signature requiring the agent, then reloads if the probe fails or the stamp is at least 45 minutes old. A failed renewal can still use a key whose remaining lifetime permits signing. Failed startup or an unusable dedicated key falls back to the session's agent. A lock left by a crashed process makes callers fall back after ten seconds. The operator can remove it after confirming its owner has exited.

## Optional user service

The wrapper starts its dedicated agent on demand. A user service can instead own that agent. Install the templates before first use, and choose one service manager. If an on-demand agent is already running, stop that dedicated process before activating the service. Never replace the session's agent. Regenerate templates after changing the configured run directory or the installed OpenSSH path.

On Linux, the operator runs:

```bash
mkdir -p ~/.config/systemd/user
ln -s ~/.local/share/devtools/signing/signing-agent.service ~/.config/systemd/user/devtools-signing-agent.service
systemctl --user daemon-reload
systemctl --user enable --now devtools-signing-agent.service
```

On macOS, the operator runs:

```bash
mkdir -p ~/Library/LaunchAgents
ln -s ~/.local/share/devtools/signing/dev.tbhb.devtools.signing.plist ~/Library/LaunchAgents/dev.tbhb.devtools.signing.plist
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/dev.tbhb.devtools.signing.plist
```

Both templates run `ssh-agent -D` on the dedicated socket with a private `umask`. The services manage the agent process without exporting `SSH_AUTH_SOCK` to other programs, while signing calls load and renew the key. Tests check generated templates as XML and with `systemd-analyze verify` where available. Activation requires the target host's service manager.

## Agent identity configuration

Keep the agent's existing author, committer, and allowed-signers settings. Add or replace these Git configuration entries, adjusting the numeric suffixes and `GIT_CONFIG_COUNT` to include existing entries. Use absolute paths for your host, and register the public signing key for the agent identity on GitHub.

For Claude Code, merge this `env` object into `~/.claude/settings.json`:

```json
{
  "env": {
    "GIT_CONFIG_COUNT": "4",
    "GIT_CONFIG_KEY_0": "gpg.format",
    "GIT_CONFIG_VALUE_0": "ssh",
    "GIT_CONFIG_KEY_1": "commit.gpgSign",
    "GIT_CONFIG_VALUE_1": "true",
    "GIT_CONFIG_KEY_2": "user.signingkey",
    "GIT_CONFIG_VALUE_2": "/home/operator/.config/devtools/signing-key.pub",
    "GIT_CONFIG_KEY_3": "gpg.ssh.program",
    "GIT_CONFIG_VALUE_3": "/home/operator/.local/bin/ssh-sign"
  }
}
```

For Codex, merge the same values into the agent's `config.toml` using [`shell_environment_policy.set`](https://developers.openai.com/codex/config-advanced/):

```toml
[shell_environment_policy.set]
GIT_CONFIG_COUNT = "4"
GIT_CONFIG_KEY_0 = "gpg.format"
GIT_CONFIG_VALUE_0 = "ssh"
GIT_CONFIG_KEY_1 = "commit.gpgSign"
GIT_CONFIG_VALUE_1 = "true"
GIT_CONFIG_KEY_2 = "user.signingkey"
GIT_CONFIG_VALUE_2 = "/home/operator/.config/devtools/signing-key.pub"
GIT_CONFIG_KEY_3 = "gpg.ssh.program"
GIT_CONFIG_VALUE_3 = "/home/operator/.local/bin/ssh-sign"
```

A `codex-agent` launcher that already supplies `~/.config/codex-agent/env.toml` can keep that identity layer and change its `gpg.ssh.program` value instead. Ensure the dedicated socket is reachable and its run directory writable under the workspace-write sandbox. Allow that specific directory through the harness's writable roots if required. The wrapper can't bypass sandbox restrictions. Use a temporary repository to sign and verify a commit from the actual agent session before retiring the old setup.

## Space-geese compatibility

After release, space-geese can point both identity setups at `~/.local/bin/ssh-sign` and migrate its provider and public-key settings into `signing.env`. It can then retire its Bash signing wrapper. `SG_*` variables and `coordinator.env` aren't read. This change doesn't require a coordinator installation. It doesn't alter the old installation automatically.

The optional `DEVTOOLS_SIGNING_HOOK` names an executable called after ensure with one argument: `ready` or `held`. Space-geese can supply an adapter that clears or creates its signing hold and emits its own events. Other tools can call the same adapter directly. The wrapper reports hook failures on stderr without blocking signing or session-agent fallback. Without a hook, the wrapper doesn't produce hold files or coordinator events.
