# Agent pull request reviews

`gh-reviewer` runs GitHub CLI with a GitHub App installation token in `GH_TOKEN`. Arguments pass through unchanged: `gh-reviewer pr review 123 --approve -R owner/repo`. It resolves `-R` or `--repo` first, otherwise the current checkout's origin. It supports HTTPS and SSH GitHub origins. Repositories must be on github.com.

## App setup

Create a GitHub App and install it on the repositories it needs to review. Grant the installation **Pull requests: read and write**, **Contents: read and write**, and **Metadata: read**. Accept permission updates in the organization after changing the App.

The test recorded in [issue #300](https://github.com/tbhb-dev/devtools/issues/300) found that GitHub recorded an approval from an installation with Contents read permission but didn't count it toward the ruleset. Raising the installation's Contents permission to write made the existing approval count. The installation needs that permission even though review tokens are down-scoped to `pull_requests: write` and `metadata: read`, with access to only the target repository. These tokens can't push. Protect the App private key as a credential capable of requesting tokens with broader installation permissions.

Generate an RSA private key for the App. Store the Privacy-Enhanced Mail (PEM) key in a 1Password **text field**: concealed fields can flatten newlines. The command also normalizes PEM whitespace, reconstructing the armor and folding the body at 64 columns before parsing PKCS1 or PKCS8 keys. JSON Web Token (JWT) signing uses Go's standard library. It doesn't invoke `openssl` or write the key to disk.

## Machine configuration

Create `~/.config/devtools/reviewer.env` with mode 600. Like [signing.env](agent-signing.md), this is data, not shell code: use literal assignments without quotes, expansion, or `export`. Environment variables override the file. `DEVTOOLS_REVIEWER_CONFIG` selects another file.

```dotenv
DEVTOOLS_REVIEWER_PROVIDER=op
DEVTOOLS_REVIEWER_APP_ID_REFERENCE=REPLACE_WITH_APP_ID_FIELD_REFERENCE
DEVTOOLS_REVIEWER_OP_REFERENCE=REPLACE_WITH_PRIVATE_KEY_FIELD_REFERENCE
DEVTOOLS_REVIEWER_TOKEN_COMMAND=/absolute/path/to/service-account-token
```

The token command is an operator-owned executable, called with no arguments, that prints the 1Password service-account token. Use the same broker pattern as `DEVTOOLS_SIGNING_TOKEN_COMMAND`, backed by the machine's approved credential store or dedicated service account. Or supply `OP_SERVICE_ACCOUNT_TOKEN` in the environment. The command refuses to invoke `op` without service-account authentication. It never uses interactive authentication. Scope the service account to the App item or its dedicated vault. Provider output and tokens aren't logged or passed as command arguments.

Optionally set `DEVTOOLS_REVIEWER_INSTALLATION_ID` to the positive installation ID to skip repository installation discovery. Otherwise the command uses `GET /repos/{owner}/{repo}/installation`. It requests tokens with `POST /app/installations/{id}/access_tokens`.

For local development, use an existing private key file and literal App ID instead:

```dotenv
DEVTOOLS_REVIEWER_PROVIDER=file
DEVTOOLS_REVIEWER_APP_ID=123456
DEVTOOLS_REVIEWER_KEY_FILE=/absolute/path/to/app-private-key.pem
```

Keep the key file private. This provider reads it into memory without making a copy.

The command caches tokens per repository and credential configuration under `~/.local/state/devtools/reviewer/`. The directory has mode 700 and token files have mode 600. Entries expire five minutes before GitHub's `expires_at`. Insecure permissions prevent the command from using cached tokens. Delete the directory to discard cached credentials after changing an App key or provider item. Ensure the state directory is writable from the agent sandbox.

## Install and migrate

Build from a devtools checkout and install explicitly:

```bash
go build -o /tmp/devtools-gh-reviewer ./cmd/gh-reviewer
/tmp/devtools-gh-reviewer --install
```

This installs `~/.local/bin/gh-reviewer`. It can replace an existing devtools reviewer binary. It refuses to overwrite the old shell wrapper or another program unless the operator supplies `--install --force`. Installation doesn't change GitHub authentication, App settings, or skill flows.

Before replacing the old wrapper, configure the provider and App installation. Back up the wrapper if needed, then run `/tmp/devtools-gh-reviewer --install --force`. The old wrapper selected a user account through `GH_CONFIG_DIR`. The new command supplies an App token instead. Existing callers of `gh-reviewer pr review` continue to work, and reviews appear under the App's bot identity. The agent's ordinary `gh` login remains separate.

Unit tests use generated keys, a fake provider executable, and an HTTP test server. They don't require live GitHub or 1Password access.
