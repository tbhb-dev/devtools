#!/usr/bin/env bats
# cspell:ignore gpgsign

setup_file() {
	local source_root
	source_root=$(git -C "$BATS_TEST_DIRNAME" rev-parse --show-toplevel)
	(cd "$source_root" && go build -o "$BATS_FILE_TMPDIR/commit-lint" ./cmd/commit-lint)
}

setup() {
	export PATH="$BATS_FILE_TMPDIR:$PATH"
	helper="$BATS_TEST_DIRNAME/../../.devtools/scripts/lint-commit-msg.sh"
	root=$(git -C "$BATS_TEST_DIRNAME" rev-parse --show-toplevel)
	local name
	for name in ${!GIT_CONFIG_@} ${!GIT_AUTHOR_@} ${!GIT_COMMITTER_@}; do
		unset "$name"
	done
	export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
	work="$BATS_TEST_TMPDIR/repo"
	mkdir -p "$work/bin"
	cd "$work" || exit 1
	git init -q
	git config user.name 'Configured Person'
	git config user.email configured@example.invalid
	export GIT_COMMITTER_NAME='Actual Committer'
	export GIT_COMMITTER_EMAIL=actual@example.invalid
	cat >bin/prek <<'STUB'
#!/usr/bin/env bash
set -eu
[[ $1 == run && $2 == --stage && $3 == commit-msg && $4 == --commit-msg-filename ]]
cp "$5" captured
printf '%s\n' "$5" > preview-path
exit "${PREVIEW_STATUS:-0}"
STUB
	chmod +x bin/prek
	export PATH="$work/bin:$PATH"
	printf 'fix: preview the draft\n\nAssisted-by: codex:gpt-6\n' >COMMIT_AGENTMSG
	cp COMMIT_AGENTMSG original
}

@test "missing sign-off uses Git's committer identity and keeps trailer order" {
	run bash "$helper"
	[ "$status" -eq 0 ]
	printf 'fix: preview the draft\n\nAssisted-by: codex:gpt-6\nSigned-off-by: Actual Committer <actual@example.invalid>\n' >expected
	cmp expected captured
}

@test "author-supplied sign-off is preserved without duplication or reordering" {
	printf 'Signed-off-by: Draft Author <author@example.invalid>\nReviewed-by: Reviewer <reviewer@example.invalid>\n' >>COMMIT_AGENTMSG
	run bash "$helper"
	[ "$status" -eq 0 ]
	cmp COMMIT_AGENTMSG captured
}

@test "preview leaves draft untouched and removes temporary file even on gate failure" {
	export PREVIEW_STATUS=7
	run bash "$helper"
	[ "$status" -eq 7 ]
	cmp original COMMIT_AGENTMSG
	[ ! -e "$(dirname "$(cat preview-path)")" ]
}

@test "real commit-msg hook rejects unsigned draft and accepts git commit -s" {
	git config core.hooksPath "$work/.git/hooks"
	git config commit.gpgsign false
	cat >.git/hooks/commit-msg <<HOOK
#!/usr/bin/env bash
"$BATS_FILE_TMPDIR/commit-lint" "\$1"
HOOK
	chmod +x .git/hooks/commit-msg
	run git -c user.name=Author -c user.email=author@example.invalid commit --allow-empty -F COMMIT_AGENTMSG
	[ "$status" -ne 0 ]
	[[ $output == *'missing required Signed-off-by'* ]]
	run git -c user.name=Author -c user.email=author@example.invalid commit --allow-empty -s -F COMMIT_AGENTMSG
	[ "$status" -eq 0 ]
	git log -1 --format=%B >actual
	grep -Fx 'Signed-off-by: Actual Committer <actual@example.invalid>' actual
	cmp original COMMIT_AGENTMSG
}

@test "preview uses the real commit buffer basename and removes its directory" {
	run bash "$helper"
	[ "$status" -eq 0 ]
	[ "$(basename "$(cat preview-path)")" = COMMIT_EDITMSG ]
	[ ! -e "$(dirname "$(cat preview-path)")" ]
}

@test "preview rejects a draft with a commit-scope Vale finding" {
	sed "s|^StylesPath = .*|StylesPath = $root/.vale|" "$root/.vale.ini" >.vale.ini
	printf 'fix: preview the draft\n\nFurthermore, use the preview.\n' >COMMIT_AGENTMSG
	cat >bin/prek <<STUB
#!/usr/bin/env bash
exec bash "$root/scripts/vale-commit-msg.sh" "\$5"
STUB
	run bash "$helper"
	[ "$status" -eq 1 ]
	[[ $output == *ai-tells.FormalTransitions* ]]
}

@test "denied committer is not injected and real gates report missing sign-off" {
	printf '[identities]\nagents = [{email = "actual@example.invalid"}]\n' >.commit-lint.toml
	cat >bin/prek <<'STUB'
#!/usr/bin/env bash
cp "$5" captured
exec commit-lint "$5"
STUB
	run bash "$helper"
	[ "$status" -eq 1 ]
	[[ $output == *'missing required Signed-off-by'* ]]
	cmp COMMIT_AGENTMSG captured
}

@test "preview stops on invalid identity configuration" {
	printf '[identities]\nagents = [{name = "Agent"}]\n' >.commit-lint.toml
	run bash "$helper"
	[ "$status" -eq 2 ]
	[ ! -e captured ]
}
