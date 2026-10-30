# Release assets

The release workflow builds binaries from the exact GitHub-signed commit beneath the SSH-signed tag. It publishes a GitHub Release associated with that tag. Failures stop the workflow. That includes a failed build or upload and a mismatch when the publisher reads the assets back. A failed run can leave a public tag or an incomplete release. Preserve that evidence and inspect the failed run before deciding what to do next.

## Release boundary

The first release whose tagged tree contains `internal/releaseassets/inventory.json` starts the asset contract. Earlier tags keep their historical tag-only contract. Don't backfill their assets or move their tags. Verification reads the inventory from the requested tag, so a newer checkout doesn't impose its inventory on an older release.

The first release still needs an owner-authorized CI dispatch after the publisher PR merges. Record its tag, workflow run URL, and successful `mise run verify-devtools-release <tag>` result in [#333](https://github.com/tbhb-dev/devtools/issues/333). That release evidence completes AC8. [#359](https://github.com/tbhb-dev/devtools/issues/359) tracks later release work.

## Inventory and archive interface

Released commands come from the canonical inventory: `agentcontext`, `agenthooks`, `agentrepo`, `agentstore`, `commit-lint`, `gh-post`, `gh-reviewer`, `guard-markdown`, and `ssh-sign`. Keep the release publisher outside that inventory.

| Target | Archive name |
| --- | --- |
| linux/amd64 | `devtools_<tag>_linux_amd64.tar.gz` |
| linux/arm64 | `devtools_<tag>_linux_arm64.tar.gz` |
| darwin/arm64 | `devtools_<tag>_darwin_arm64.tar.gz` |
| windows/amd64 | `devtools_<tag>_windows_amd64.zip` |

Replace `<tag>` with the full version tag, including its leading `v`. Each archive contains every released command at its root. Unix entries have executable permissions. Windows entries keep `.exe`. This layout works with [mise's GitHub backend](https://mise.jdx.dev/dev-tools/backends/github.html). Consumer pin and lock migration belongs to [#544](https://github.com/tbhb-dev/devtools/issues/544).

## Build evidence

`release.json` records `version`, the full tagged `commit`, the UTC `build_date`, and an `inventory` object containing `platforms` and `binaries`. `SHA256SUMS` covers the archives and metadata, with lowercase SHA-256 digests, spaces separating each digest from its filename, and entries sorted by filename. It doesn't include itself.

The builder uses `CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`, and the shared linker flags from `.devtools/scripts/go-ldflags.sh`. Those flags clear the build ID and derive build metadata from the release commit. Archive timestamps use that commit's date as well. Reproducing identical bytes requires the same pinned Go toolchain.

## Publication and verification

Content creation passes through `gh-post` under the release App's identity. Uploads never replace an existing asset. A retry downloads and compares existing assets before uploading missing evidence. Matching bytes pass. Different bytes, unexpected assets, duplicate names, failed downloads, and failed uploads stop publication. After uploading, the publisher downloads the evidence again and compares it against the local build.

Run `mise run verify-devtools-release <tag>` after fetching origin and tags. The verifier retains the tag, commit, signature, identity, and ancestry checks. For an asset-bearing release it also downloads the requested assets and recomputes their checksums. Metadata must match the tagged inventory and commit date. Each archive must contain the complete binary inventory. Missing evidence, corrupt archives, unexpected entries, and metadata mismatches fail verification.

The publisher doesn't recover or replace assets automatically. Don't move or delete a tag to conceal a partial publication. Durable recovery belongs to [#359](https://github.com/tbhb-dev/devtools/issues/359).

## Development installation

Source installation remains available. Use `mise run install` from a development checkout, or `go install github.com/tbhb-dev/devtools/cmd/<command>@<ref>` for a source ref. Released archives let consumers install commands without compiling them locally.
