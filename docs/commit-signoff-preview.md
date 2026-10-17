# Previewing a commit sign-off

Run `mise run lint-commit-msg` to check `COMMIT_AGENTMSG` before committing with `git commit -s -F COMMIT_AGENTMSG`. The task runs the commit-msg gates against a temporary copy and leaves the draft unchanged, including when a gate fails.

When the draft has no sign-off trailer, the preview appends `Signed-off-by` using `git var GIT_COMMITTER_IDENT`, the identity Git resolves for `-s`, so environment overrides take precedence over repository configuration. Existing trailers keep their order. If the draft already contains a sign-off trailer, the preview preserves it without adding another. Git may still append the current committer's sign-off when it differs from the existing one.

The real commit-msg hook still requires a sign-off. Previewing doesn't change the hook or make an unsigned commit pass. Use the same Git identity settings for the preview and the commit.

Consumers inherit this behavior through the `devtools:lint-commit-msg` task template and its helper in the vendored payload. Squash-message lint continues to check the supplied message directly.
