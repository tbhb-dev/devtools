# Previewing a commit sign-off

Run `mise run lint-commit-msg` to check `COMMIT_AGENTMSG` before committing with `git commit -s -F COMMIT_AGENTMSG`. The task runs the commit-msg gates against a temporary copy and leaves the draft unchanged, including when a gate fails.

When the draft has no sign-off trailer, the preview appends `Signed-off-by` using the optional machine `signoff_identity`. Without that setting, it falls back to `git config user.name` and `user.email`. It checks either identity through `commit-lint --check-signoff` before adding it. A denied Git fallback stops the preview and points the operator to the machine setting, which the operator must configure when Git names an agent. Configuration errors also stop the preview. Existing trailers keep their order. If the draft already contains a sign-off trailer, the preview preserves it without adding another. Git may still append the current committer's sign-off when it differs from the existing one.

The real commit-msg hook still requires a sign-off. Previewing doesn't change the hook or make an unsigned commit pass.

Consumers inherit this behavior through the `devtools:lint-commit-msg` task template and its helper in the vendored payload. Squash-message lint continues to check the supplied message directly.

The helper probes `agentrepo commit-preview --probe` using the consumer-selected binary. When supported, `agentrepo commit-preview` runs the preview. Older or absent binaries use the frozen shell fallback throughout the N-2 compatibility window. A failed Go preview returns its status without rerunning the shell preview.
