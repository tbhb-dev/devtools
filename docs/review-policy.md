# Repository review policy

Commit, pull request description, and squash message reviews accept optional free-form criteria in `.agents/review-instructions.md`. This path lives outside the deployed skill directories. Repositories without this file keep the built-in review rules. Mechanical leak scans remain separate and keep their existing rules.

## Policy authority

The shared reader in `packages/agents/source/lib/lib.sh` reads the file from the local `origin/<default>` remote tracking ref, never from the working tree, index, or branch under review. Refresh origin before reviewing to get current policy. Default selection comes from `refs/remotes/origin/HEAD`. Without that symbolic ref, the reader uses main, or master when only that local default exists. Applying policy requires a trusted remote tracking commit. Only a regular, non-executable Git blob qualifies as the file.

## Review precedence

The policy overrides conflicting built-in review criteria. A credentials floor follows it in the prompt and always applies: reviewers must still check keys, tokens, `OAuth` client secrets, private keys, and passwords and report exposed credentials. Neither policy text nor an operator note can waive that floor.

Policy supplies review text only. It can't select a reviewer, executable, command, tool, or authority. Reviewer configuration stays on the machine.

A repository managing network policy could adopt this text:

```markdown
Tailnet and RFC 1918 addresses, subnet routes, tags, and member logins in `policy.json` are expected configuration. Allow these network values. Continue to report credentials and secrets.
```

## Policy changes under review

When the reviewed tree changes the policy path relative to the branch's merge base with the trusted default, the entire review uses built-in rules alone. This includes adding or deleting the file and policy changes committed earlier on the branch. An unstaged edit can't supply instructions. Reviews also use built-in rules alone when Git can't establish a common ancestor. Adopt policy changes through ordinary review and the repository's human approval process before using them on later branches. Repository owners may add a CODEOWNERS rule for this path.

## Audit records

Every scripted review reports the policy path, SHA-256 digest of the applied bytes, status, and trusted ref and commit, while the review stamp records the digest, status, ref, and commit. An absent or suppressed policy reports a digest of `none`. Moving the trusted ref during review prevents stamping. Moving it after review requires a new review before committing or publishing.

The same reader and prompt precedence serve commit, PR, and merge reviews on both harnesses.
