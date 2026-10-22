# Gitleaks scans and fixture policy

Copy `.devtools/config/gitleaks.toml` to `.gitleaks.toml` at the repository root. The `devtools:gitleaks` task runs from that root, where gitleaks discovers the file automatically. The template extends the built-in rules with `useDefault = true`, which you must keep to preserve secret detection when adding an exception. Exceptions aren't active in the template, but its commented example allows only `tests/fixtures/private-key.pem`, a synthetic private key used to test private-key detection. Remove the comment markers from the example only when that exact fixture exists and contains no real credentials. Replace the example path and justification if your fixture serves a different purpose.

Treat changes to `.gitleaks.toml` as security configuration. Review the fixture contents and explain why you must suppress detection there. Anchor each path expression with `^` and `$`, escape literal dots, and specify the individual file. Don't allow entire test directories, extensions, or secret patterns across the repository: a path exception suppresses every secret rule for the matching file.

Never put real credentials in an allowed fixture.

Run `mise run devtools:gitleaks -- --mode ci` after committing the policy and fixtures: the task scans git history, so an uncommitted fixture doesn't exercise it. Confirm that the synthetic fixture passes and that a generated secret outside the allowed path still fails. Keep scanner output redacted. This policy is separate from the private identifier gate's `.leak-allow` file.

## Scan boundaries

`mise run devtools:gitleaks` defaults to `--mode ci`. Environment variables don't select the mode. Unknown arguments or modes fail. Only local mode accepts `--base`.

| Caller | Explicit mode and base |
| --- | --- |
| Security and check-all aggregates | `--mode ci` |
| Reusable workflow and fix-pr diagnostic | `--mode ci` |
| Commit gates for normal and amended commits | `--mode local` with the origin default |
| PR creation and updates | `--mode local --base refs/remotes/origin/<declared-base>` |

Unattended entry points use the same publication gates. Merge uses existing CI results and squash-message gates without another secret scan.

### CI history

CI scans all refs with `gitleaks git --verbose --redact .`. The workflow keeps its triggers and full-history checkout, with findings blocking the job. Reproducing a CI failure requires the same fetched refs and complete history. A secret on an unrelated pushed branch or tag can fail the scan regardless of the checked-out ref.

### Local history and index

Local mode captures `HEAD`, the base commit, and the full index tree before scanning. History selection is exactly `base_oid..head_oid`, with full history and merge diffs against the first parent. Removing a secret later on the branch doesn't remove its finding. Other refs can't expand that range.

A separate snapshot commit exposes every indexed file through its root patch. It has no parent. This covers unchanged files as well as partially staged bytes. Scanning excludes untracked files and reads indexed content even when the working file differs.

Submodule contents remain separate repositories.

### Required origin base

Without `--base`, local mode requires the symbolic ref `refs/remotes/origin/HEAD` to target an origin branch. An explicit base must specify a fully qualified branch ref under `refs/remotes/origin/`. Names can contain slashes. The scanner has no local default, upstream, or guessed fallback. PR publication checks that this exact origin commit matches its review stamp.

Missing or broken origin refs, unborn `HEAD`, shallow history, and histories without a common ancestor block the scan. Fetch complete origin history or repair the default symbolic ref before retrying. The scanner never fetches, and its output identifies the mode and captured commits to make stale origin refs visible.

### Scan validation

Snapshot plumbing uses fixed metadata without signing. It doesn't create a ref or change `HEAD`, refs, index entries, or working files. Snapshot objects remain for normal Git garbage collection. Git and scanner failures block the gate, even when the scanner logs an error but exits zero. A zero-commit result also fails when Git expects patch content. Independent validation lets empty ranges, empty index trees, and histories without patch content pass.

### Coverage and review

Local gates deliberately stop scanning secrets found only in unrelated refs or historical base commits absent from the index. This reduces local security coverage. CI still blocks on detectable secrets reachable through fetched refs. The private-strings gate doesn't replace that history coverage.

Before merge, the repository owner must approve the change's current head and explicitly acknowledge the reduction. Existing owner-review and merge-policy requirements still apply.

Both modes keep redaction and report the rule, repository-relative path, and line. Redacted verbose metadata remains available.

A `gitleaks:allow` marker requires human review. Reviewers must explain why the synthetic value needs suppression. Upstream accepts markers with or without explanatory text, so the scanner doesn't enforce justification.
