#!/usr/bin/env bats
# cspell:ignore gpgsign

setup_file() {
	local source_root
	source_root=$(git -C "$BATS_TEST_DIRNAME" rev-parse --show-toplevel)
	(cd "$source_root" && go build -o "$BATS_FILE_TMPDIR/commit-lint" ./cmd/commit-lint && go build -o "$BATS_FILE_TMPDIR/agentrepo" ./cmd/agentrepo)
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
	export XDG_CONFIG_HOME="$BATS_TEST_TMPDIR/config"
	unset DEVTOOLS_SIGNOFF_IDENTITY
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

@test "missing sign-off falls back to Git's configured identity and keeps trailer order" {
	run bash "$helper"
	[ "$status" -eq 0 ]
	printf 'fix: preview the draft\n\nAssisted-by: codex:gpt-6\nSigned-off-by: Configured Person <configured@example.invalid>\n' >expected
	cmp expected captured
}

@test "configured sign-off identity wins over Git identity" {
	export DEVTOOLS_SIGNOFF_IDENTITY='Responsible Person <responsible@example.invalid>'
	run bash "$helper"
	[ "$status" -eq 0 ]
	grep -Fx 'Signed-off-by: Responsible Person <responsible@example.invalid>' captured
	! grep -q 'Signed-off-by: Configured Person' captured
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

@test "preview accepts 72-character body lines and rejects 73" {
	cat >bin/prek <<STUB
#!/usr/bin/env bash
exec "$BATS_FILE_TMPDIR/commit-lint" --rules shape "\$5"
STUB
	chmod +x bin/prek
	printf 'fix: preview the draft\n\nThis body line deliberately reaches the configured body line width limit\n' >COMMIT_AGENTMSG
	run bash "$helper"
	[ "$status" -eq 0 ]
	printf 'fix: preview the draft\n\nThis body line deliberately reaches the configured body line width limit.\n' >COMMIT_AGENTMSG
	run bash "$helper"
	[ "$status" -eq 1 ]
	[[ $output == *'body line 3 exceeds 72 chars (73)'* ]]
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

@test "denied Git identity refuses with configuration guidance" {
	printf '[identities]\nagents = [{email = "configured@example.invalid"}]\n' >.commit-lint.toml
	run bash "$helper"
	[ "$status" -eq 1 ]
	grep -Fq 'denied agent sign-off' <<<"$output"
	grep -Fq 'set signoff_identity in ~/.config/devtools/<repository>.conf or ~/.config/devtools/devtools.conf' <<<"$output"
	[ ! -e captured ]
}

@test "denied configured sign-off identity refuses" {
	export DEVTOOLS_SIGNOFF_IDENTITY='Agent <agent@example.invalid>'
	printf '[identities]\nagents = [{email = "agent@example.invalid"}]\n' >.commit-lint.toml
	run bash "$helper"
	[ "$status" -eq 1 ]
	grep -Fq 'denied agent sign-off' <<<"$output"
	[ ! -e captured ]
}

@test "preview stops on invalid identity configuration" {
	printf '[identities]\nagents = [{name = "Agent"}]\n' >.commit-lint.toml
	run bash "$helper"
	[ "$status" -eq 2 ]
	[ ! -e captured ]
}

@test "capable selected binary probes without arguments then owns operational failure" {
	cat >bin/agentrepo <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >>calls
if [[ $* == 'commit-preview --probe' ]]; then exit 0; fi
printf 'selected stdout\n'
printf 'selected stderr\n' >&2
exit 7
STUB
	chmod +x bin/agentrepo
	run bash "$helper" 'literal argument' --unexpected
	[ "$status" -eq 7 ]
	[[ $output == *'selected stdout'* && $output == *'selected stderr'* ]]
	printf 'commit-preview --probe\ncommit-preview literal argument --unexpected\n' >expected
	cmp expected calls
	[ ! -e captured ]
}

@test "old or unsupported selected binary falls back without consulting later binaries" {
	mkdir -p later
	cat >bin/agentrepo <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >>calls
printf 'unsupported\n' >&2
exit 5
STUB
	cat >later/agentrepo <<'STUB'
#!/usr/bin/env bash
exit 99
STUB
	chmod +x bin/agentrepo later/agentrepo
	export PATH="$work/bin:$work/later:$PATH"
	run bash "$helper" ignored
	[ "$status" -eq 0 ]
	[ -z "$output" ]
	printf 'commit-preview --probe\n' >expected
	cmp expected calls
	cmp original COMMIT_AGENTMSG
	[ -e captured ]
}

@test "absent binary uses the frozen fallback" {
	local tool shell
	shell=$(command -v bash)
	mkdir tools
	for tool in git cp awk grep mktemp rm dirname commit-lint prek; do
		ln -s "$(command -v "$tool")" "tools/$tool"
	done
	ln -s "$shell" tools/bash
	export PATH="$work/tools"
	run "$shell" "$helper"
	[ "$status" -eq 0 ]
	[ -e captured ]
	"$work/tools/cp" COMMIT_AGENTMSG after
}
