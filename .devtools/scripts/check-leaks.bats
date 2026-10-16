#!/usr/bin/env bats
# The leak check against throwaway repositories. Every sample is built at
# run time from made-up values, so no real private value appears in this
# file, and the real O11Y_* environment is cleared before each case.
# cspell:ignore compgen gpgsign btrfs servicesx nothex repointed gitlink cacheinfo

setup() {
	checker="$BATS_TEST_DIRNAME/check-leaks.sh"
	# The scheme separator, kept out of the source so a search for it finds only real uses.
	op_slashes=/
	repo_root=$(git -C "$BATS_TEST_DIRNAME" rev-parse --show-toplevel)

	unset GIT_DIR GIT_COMMON_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY
	unset PRE_COMMIT_FROM_REF PRE_COMMIT_TO_REF GITHUB_ACTIONS CI
	for name in $(compgen -v O11Y_); do unset "$name"; done

	export O11Y_PRIVATE_STRINGS="zebra-host-01, /zq/stash/dir, /zq/bin/tsc, quokka-pond, 203.0.113.77, kestrel99, mango-secret"

	work=$(mktemp -d)
	cd "$work" || exit 1
	git init -q -b main .
	git config user.name tester
	git config user.email tester@example.invalid
	git config commit.gpgsign false
	git config tag.gpgsign false
	git config core.hooksPath /dev/null
	printf '%s\n' ignored.txt >.gitignore
	git add .gitignore
	git commit -q -m "chore: start"
	base=$(git rev-parse HEAD)
	printf '%s\n' '^\.devtools/' '^\.claude/' '^\.agents/' '^\.vale/' '^\.config/mise/conf\.d/' '^tools/check-leaks\.bats$' >.leak-allow
}

teardown() {
	rm -rf "$work"
}

# no_value: the output must not carry the value anywhere.
no_value() {
	[[ $output != *"$1"* ]]
}

# Every made-up value the cases plant. run_check asserts after each run
# that none of them appears in the output, so redaction is tested in every
# case, not only in the ones that remember to.
secrets=(zebra-host-01 /zq/stash/dir /zq/bin/tsc quokka-pond 203.0.113.77 kestrel99
	mango-secret wombat-site wombat-node 10.1.2.3 100.100.1.2 100.64.0.9 100.127.255.1 fd7a:115c:a1e0
	172.16.0.5 172.31.9.9 192.168.1.1 other-example box.other-net foo.ts.net)

no_secrets() {
	local v
	for v in "${secrets[@]}"; do
		if [[ $output == *"$v"* ]]; then
			echo "the output repeats a planted value (index of it: ${v:0:3}...)" >&2
			return 1
		fi
	done
}

run_check() {
	run bash "$checker" "$@"
	no_secrets
}

@test "a clean tree passes silently" {
	echo "nothing private here" >clean.md
	git add clean.md
	run_check
	[ "$status" -eq 0 ]
	[ -z "$output" ]
}

@test "literals match regardless of case" {
	echo "Host ZEBRA-Host-01" >leak.md
	git add leak.md
	run_check
	[ "$status" -eq 1 ]
	[[ $output == *"leak.md:1: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
	no_value "ZEBRA-Host-01"
}

@test "a value under four characters is skipped with a warning" {
	export O11Y_PRIVATE_STRINGS="ab,kestrel99"
	echo "ab is everywhere, abc too" >short.md
	git add short.md
	run_check
	[ "$status" -eq 0 ]
	[[ $output == *"O11Y_PRIVATE_STRINGS entry 1 is under 4 characters and is skipped"* ]]
}

@test "an empty private-strings list works" {
	export O11Y_PRIVATE_STRINGS=""
	echo "fine" >ok.md
	run_check
	[ "$status" -eq 0 ]
	[[ $output == *"no private values loaded"* ]]
}

@test "each static pattern is a finding that prints no text" {
	local cases=(
		"op:$op_slashes/vault/item/field|password-manager reference"
		"ssh box.other-net.ts.net|tailnet host name"
		"peer 100.100.1.2 up|tailnet address"
		"peer 100.64.0.9 up|tailnet address"
		"peer 100.127.255.1 up|tailnet address"
		"gw 10.1.2.3|private IPv4 address"
		"gw 172.16.0.5|private IPv4 address"
		"gw 172.31.9.9|private IPv4 address"
		"gw 192.168.1.1|private IPv4 address"
	)
	local item text why
	unset O11Y_SSH_HOST O11Y_REMOTE_DIR O11Y_REMOTE_TAILSCALE O11Y_TAILNET_DOMAIN O11Y_NODE_ADDR
	for item in "${cases[@]}"; do
		text=${item%%|*}
		why=${item#*|}
		echo "$text" >leak.md
		git add leak.md
		run_check
		[ "$status" -eq 1 ]
		[[ $output == *"leak.md:1: pattern: $why"* ]]
		no_value "${text#* }"
	done
}

@test "addresses and paths outside the private shapes pass" {
	export O11Y_NODE_ADDR="" O11Y_PRIVATE_STRINGS=""
	printf '%s\n' "doc 192.0.2.10 and 198.51.100.4" "pub 100.63.0.1 and 100.128.0.1" \
		"pub 172.15.0.1 and 172.32.0.1 and 11.1.2.3" "dir /srv/o11y and /volume/x" >ok.md
	git add ok.md
	run_check
	[ "$status" -eq 0 ]
}

@test "example.ts.net and its subdomains are exempt, other tailnet names are not" {
	printf '%s\n' "https://grafana.example.ts.net and example.ts.net" >ok.md
	git add ok.md
	run_check
	[ "$status" -eq 0 ]
	echo "other-example.ts.net" >leak.md
	git add leak.md
	run_check
	[ "$status" -eq 1 ]
	[[ $output == *"leak.md:1: pattern: tailnet host name"* ]]
}

@test "vendored trees are exempt from patterns but not from literals" {
	mkdir -p .devtools .claude/skills .vale .config/mise/conf.d tools
	local dir
	for dir in .devtools .claude/skills .vale .config/mise/conf.d; do
		echo "see 10.1.2.3 and op:$op_slashes/a/b/c and /volume1/x" >"$dir/vendored.md"
	done
	echo "pattern 10.1.2.3" >tools/check-leaks.bats
	git ls-files --others --exclude-standard -z | xargs -0 git add --
	run_check
	[ "$status" -eq 0 ]
	echo "host zebra-host-01" >.devtools/literal.md
	git ls-files --others --exclude-standard -z | xargs -0 git add --
	run_check
	[ "$status" -eq 1 ]
	[[ $output == *".devtools/literal.md:1: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
}

@test "--staged catches a private value staged while the working file is clean" {
	echo "zebra-host-01" >leak.md
	git add leak.md
	: >leak.md
	run_check --staged
	[ "$status" -eq 1 ]
	[[ $output == *"leak.md:1: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
	no_value "zebra-host-01"
	# The tree scan reads the working file, so it is the staged scan that matters.
	run_check
	[ "$status" -eq 0 ]
}

@test "--staged ignores a private value that is only in the working file" {
	echo "fine" >leak.md
	git add leak.md
	echo "zebra-host-01" >leak.md
	run_check --staged
	[ "$status" -eq 0 ]
	run_check
	[ "$status" -eq 1 ]
}

@test "--staged scans only staged changes" {
	echo "zebra-host-01" >old.md
	git add old.md
	git commit -q -m "chore: old"
	echo "fine" >new.md
	git add new.md
	run_check --staged
	[ "$status" -eq 0 ]
}

@test "untracked files count, ignored files do not" {
	echo "zebra-host-01" >ignored.txt
	run_check
	[ "$status" -eq 0 ]
	echo "zebra-host-01" >untracked.md
	run_check
	[ "$status" -eq 1 ]
	[[ $output == *"untracked.md:1:"* ]]
	[[ $output != *"ignored.txt"* ]]
}

@test "a tracked file deleted from the working tree does not break the scan" {
	echo "fine" >gone.md
	git add gone.md
	git commit -q -m "chore: gone"
	rm gone.md
	run_check
	[ "$status" -eq 0 ]
}

@test "binary files are searched as raw bytes, staged and in a range" {
	printf 'head\000zebra-host-01\000tail' >blob.bin
	printf '\000gw 10.1.2.3\000' >net.bin
	git add blob.bin net.bin
	run_check --staged
	[ "$status" -eq 1 ]
	[[ $output == *"blob.bin:binary: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
	[[ $output == *"net.bin:binary: pattern: private IPv4 address"* ]]
	git commit -q -m "chore: add binaries"
	run_check --range "$base..HEAD"
	[ "$status" -eq 1 ]
	[[ $output == *"blob.bin:2: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
	run_check
	[ "$status" -eq 1 ]
	[[ $output == *"blob.bin:binary: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
}

@test "a clean binary file passes" {
	printf 'head\000tail\001' >blob.bin
	git add blob.bin
	run_check --staged
	[ "$status" -eq 0 ]
}

@test "--range catches a private value in an added line" {
	echo "fine" >a.md
	git add a.md
	git commit -q -m "chore: add a"
	printf 'fine\nline two zebra-host-01\n' >a.md
	git commit -q -am "chore: edit a"
	run_check --range "$base..HEAD"
	[ "$status" -eq 1 ]
	[[ $output == *"a.md:2: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
	no_value "zebra-host-01"
}

@test "--range catches a private value in a commit message" {
	echo "fine" >a.md
	git add a.md
	git commit -q -m "chore: add a" -m "the box is zebra-host-01 on the tailnet"
	run_check --range "$base..HEAD"
	[ "$status" -eq 1 ]
	[[ $output =~ commit\ [0-9a-f]{7}:[0-9]+:\ private\ literal\ O11Y_PRIVATE_STRINGS\ entry\ 1 ]]
	no_value "zebra-host-01"
}

@test "--range catches a ts.net host name in a commit message" {
	echo "fine" >a.md
	git add a.md
	git commit -q -m "chore: reach foo.ts.net"
	run_check --range "$base..HEAD"
	[ "$status" -eq 1 ]
	[[ $output =~ commit\ [0-9a-f]{7}:[0-9]+:\ pattern:\ tailnet\ host\ name ]]
}

@test "--range ignores removed lines and lines outside the range" {
	echo "zebra-host-01" >a.md
	git add a.md
	git commit -q -m "chore: old leak"
	old=$(git rev-parse HEAD)
	echo "fine" >a.md
	git commit -q -am "chore: clean it"
	run_check --range "$old..HEAD"
	[ "$status" -eq 0 ]
}

@test "--range reports a bad range as misuse" {
	run_check --range missing..HEAD
	[ "$status" -eq 2 ]
}

@test "a private string in a tracked file name is a finding" {
	echo "clean content" >zebra-host-01.md
	git add zebra-host-01.md
	run_check
	[ "$status" -eq 1 ]
	[[ $output =~ path#[0-9a-f]{8}:path:\ private\ literal\ O11Y_PRIVATE_STRINGS\ entry\ 1 ]]
}

@test "a staged empty or binary file with a private name is a finding" {
	: >empty-zebra-host-01.txt
	printf '\000\001' >bin-kestrel99.bin
	git add empty-zebra-host-01.txt bin-kestrel99.bin
	run_check --staged
	[ "$status" -eq 1 ]
	[[ $output =~ path#[0-9a-f]{8}:path:\ private\ literal\ O11Y_PRIVATE_STRINGS\ entry\ 1 ]]
	[[ $output =~ path#[0-9a-f]{8}:path:\ private\ literal\ O11Y_PRIVATE_STRINGS\ entry\ 1 ]]
	no_value "zebra-host-01"
	no_value "kestrel99"
}

@test "a directory name counts and a path pattern applies to it" {
	mkdir -p 10.1.2.3 dir
	echo x >10.1.2.3/a.md
	echo x >dir/10.1.2.3.md
	git add 10.1.2.3 dir
	run_check --staged
	[ "$status" -eq 1 ]
	[[ $output =~ path#[0-9a-f]{8}:path:\ pattern:\ private\ IPv4\ address ]]
	[[ $output =~ path#[0-9a-f]{8}:path:\ pattern:\ private\ IPv4\ address ]]
	no_value "10.1.2.3"
}

@test "a rename that changes no line still shows its new name in a range" {
	echo "some content" >plain.md
	git add plain.md
	git commit -q -m "chore: add plain"
	before=$(git rev-parse HEAD)
	git mv plain.md zebra-host-01.md
	git commit -q -m "chore: rename"
	run_check --range "$before..HEAD"
	[ "$status" -eq 1 ]
	[[ $output =~ tree\ [0-9a-f]{7}:[0-9]+:\ private\ literal\ O11Y_PRIVATE_STRINGS\ entry\ 1 ]]
}

@test "a mode change or an empty file in a range still shows its path" {
	: >kestrel99.txt
	git add kestrel99.txt
	git commit -q -m "chore: empty file"
	run_check --range "$base..HEAD"
	[ "$status" -eq 1 ]
	[[ $output =~ path#[0-9a-f]{8}:path:\ private\ literal\ O11Y_PRIVATE_STRINGS\ entry\ 1 ]]
	no_value "kestrel99"
}

@test "vendored paths skip the patterns but not the literals" {
	mkdir -p .devtools
	: >.devtools/10.1.2.3.md
	: >.devtools/zebra-host-01.md
	git add .devtools
	run_check --staged
	[ "$status" -eq 1 ]
	[[ $output =~ path#[0-9a-f]{8}:path:\ private\ literal ]]
	[[ $output != *"pattern"* ]]
	[[ $output != *"10.1.2.3"* ]]
}

@test "file arguments scan exactly those files" {
	echo "zebra-host-01" >leak.md
	echo "fine" >ok.md
	git add ok.md
	run_check ok.md
	[ "$status" -eq 0 ]
	run_check ok.md leak.md
	[ "$status" -eq 1 ]
	[[ $output == *"leak.md:1: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
	[[ $output != *"ok.md:"* ]]
	run_check missing.md
	[ "$status" -eq 2 ]
}

@test "a commit message file is scanned like any file" {
	printf 'feat: add thing\n\nreaches foo.ts.net\n' >msg
	run_check msg
	[ "$status" -eq 1 ]
	[[ $output == *"msg:3: pattern: tailnet host name"* ]]
}

@test "with no private values loaded it says so and checks the patterns only" {
	for name in $(compgen -v O11Y_); do unset "$name"; done
	echo "zebra-host-01" >ok.md
	git add ok.md
	run_check
	[ "$status" -eq 0 ]
	[[ $output == *"no private values loaded"* ]]
	[[ $output == *"only the publishable patterns are checked"* ]]
	echo "op:$op_slashes/a/b/c" >leak.md
	git add leak.md
	run_check
	[ "$status" -eq 1 ]
	[[ $output == *"leak.md:1: pattern: password-manager reference"* ]]
}

@test "in GitHub Actions the static-only notice is also an annotation" {
	for name in $(compgen -v O11Y_); do unset "$name"; done
	GITHUB_ACTIONS=true run_check
	[ "$status" -eq 0 ]
	[[ $output == *"::notice title=leak check::no private values loaded"* ]]
}

@test "an unknown option is misuse" {
	run_check --bogus
	[ "$status" -eq 2 ]
}

@test "the notice says when mise.local.toml is present but its values did not load" {
	for name in $(compgen -v O11Y_); do unset "$name"; done
	run_check
	[[ $output != *"mise.local.toml exists"* ]]
	: >mise.local.toml
	run_check
	[ "$status" -eq 0 ]
	[[ $output == *"mise.local.toml exists here but its values are not in the environment"* ]]
}

# --- merges and type changes -------------------------------------------

@test "--range scans what a merge adds beyond its parents" {
	printf 'one\ntwo\nthree\n' >m.md
	git add m.md
	git commit -q -m "chore: m"
	start=$(git rev-parse HEAD)
	git switch -q -c side
	printf 'one\ntwo\nthree\nside\n' >m.md
	git commit -q -am "chore: side"
	git switch -q main
	printf 'top\none\ntwo\nthree\n' >m.md
	git commit -q -am "chore: top"
	git merge -q --no-commit --no-ff side
	printf 'top\none\ntwo\nzebra-host-01\nthree\nside\n' >m.md
	git add m.md
	git commit -q -m "chore: merge"
	run_check --range "$start..HEAD"
	[ "$status" -eq 1 ]
	[[ $output == *"m.md:4: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
}

@test "--range passes a clean merge" {
	printf 'one\n' >m.md
	git add m.md
	git commit -q -m "chore: m"
	start=$(git rev-parse HEAD)
	git switch -q -c side
	echo side >s.md
	git add s.md
	git commit -q -m "chore: side"
	git switch -q main
	echo top >t.md
	git add t.md
	git commit -q -m "chore: top"
	git merge -q --no-ff -m "chore: merge" side
	run_check --range "$start..HEAD"
	[ "$status" -eq 0 ]
}

@test "a regular file replaced by a symlink to a private value is caught staged and in a range" {
	echo "plain" >l
	git add l
	git commit -q -m "chore: plain"
	before=$(git rev-parse HEAD)
	rm l
	ln -s zebra-host-01 l
	git add l
	run_check --staged
	[ "$status" -eq 1 ]
	[[ $output == *"l:1: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
	git commit -q -m "chore: link"
	run_check --range "$before..HEAD"
	[ "$status" -eq 1 ]
	[[ $output == *"l:1: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
}

# --- paths --------------------------------------------------------------

@test "both sides of a rename are scanned in a staged change" {
	echo "some content" >zebra-host-01.txt
	git add zebra-host-01.txt
	git commit -q -m "chore: old name"
	git mv zebra-host-01.txt clean-name.txt
	run_check --staged
	[ "$status" -eq 1 ]
	[[ $output =~ path#[0-9a-f]{8}:path:\ private\ literal\ O11Y_PRIVATE_STRINGS\ entry\ 1 ]]
}

@test "a staged rename to a private name is caught" {
	echo "some content" >plain.txt
	git add plain.txt
	git commit -q -m "chore: plain"
	git mv plain.txt zebra-host-01.txt
	run_check --staged
	[ "$status" -eq 1 ]
	[[ $output =~ path#[0-9a-f]{8}:path:\ private\ literal\ O11Y_PRIVATE_STRINGS\ entry\ 1 ]]
}

@test "filenames with newlines or spaces can't corrupt the scan" {
	echo x >"$(printf 'a\nzebra-host-01')"
	echo x >"clean name with spaces.md"
	echo x >"$(printf 'b\nclean')"
	git add .
	run_check --staged
	[ "$status" -eq 1 ]
	[ "$(grep -c 'private literal' <<<"$output")" -eq 1 ]
	rm -f a* b*
	git rm -q --cached -f a* b* 2>/dev/null || true
	run_check --staged
	[ "$status" -eq 0 ]
}

@test "a clean newline filename passes in tree and range modes" {
	echo x >"$(printf 'b\nclean')"
	git add .
	git commit -q -m "chore: odd name"
	run_check
	[ "$status" -eq 0 ]
	run_check --range "$base..HEAD"
	[ "$status" -eq 0 ]
}

# --- disclosure ---------------------------------------------------------

@test "a path that matches a rule is replaced by an opaque identifier" {
	echo "gw 10.1.2.3" >zebra-host-01.txt
	git add zebra-host-01.txt
	run_check --staged
	[ "$status" -eq 1 ]
	[[ $output =~ path#[0-9a-f]{8}:path:\ private\ literal\ O11Y_PRIVATE_STRINGS\ entry\ 1 ]]
	[[ $output =~ path#[0-9a-f]{8}:1:\ pattern:\ private\ IPv4\ address ]]
	[[ $output != *".txt"* ]]
}

@test "errors don't repeat the argument that caused them" {
	run_check missing-zebra-host-01.md
	[ "$status" -eq 2 ]
	run_check --zebra-host-01
	[ "$status" -eq 2 ]
	run_check --range zebra-host-01..HEAD
	[ "$status" -eq 2 ]
}

# --- failures -----------------------------------------------------------

@test "a corrupt index fails the staged check instead of passing it" {
	echo "fine" >a.md
	git add a.md
	echo "junk" >.git/index
	run_check --staged
	[ "$status" -eq 2 ]
	[[ $output == *"incomplete"* ]]
}

@test "outside a git repository the check fails" {
	cd "$BATS_TEST_TMPDIR"
	run_check --staged
	[ "$status" -eq 2 ]
}

# --- the NAS roots ------------------------------------------------------

@test "an unreadable file fails the check without printing its name" {
	echo "fine" >zebra-host-01.txt
	chmod 000 zebra-host-01.txt
	run_check zebra-host-01.txt
	# A superuser can read it anyway, which leaves nothing to fail.
	[ "$status" -ne 0 ] || [ -r zebra-host-01.txt ]
}

# --- diffs git suppresses ------------------------------------------------

@test "a text blob that replaces a binary one is scanned though git hides the diff" {
	printf 'clean\000binary' >f.dat
	git add f.dat
	git commit -q -m "chore: binary"
	before=$(git rev-parse HEAD)
	printf 'zebra-host-01\n' >f.dat
	git commit -q -am "chore: now text"
	run_check --range "$before..HEAD"
	[ "$status" -eq 1 ]
	[[ $output == *"f.dat:1: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
}

@test "a file marked -diff in .gitattributes is scanned whole" {
	echo '*.dat -diff' >.gitattributes
	echo "zebra-host-01" >leak.dat
	git add .gitattributes leak.dat
	git commit -q -m "chore: hidden diff"
	run_check --range "$base..HEAD"
	[ "$status" -eq 1 ]
	[[ $output == *"leak.dat:1: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
}

@test "a merge result whose diff git hides is scanned whole" {
	echo '*.dat -diff' >.gitattributes
	echo one >m.dat
	git add .gitattributes m.dat
	git commit -q -m "chore: base dat"
	start=$(git rev-parse HEAD)
	git switch -q -c side
	printf 'one\nside\n' >m.dat
	git commit -q -am "chore: side"
	git switch -q main
	printf 'top\none\n' >m.dat
	git commit -q -am "chore: top"
	git merge -q --no-commit --no-ff side || true
	printf 'top\none\nside\nzebra-host-01\n' >m.dat
	git add m.dat
	git commit -q -m "chore: merge"
	run_check --range "$start..HEAD"
	[ "$status" -eq 1 ]
	[[ $output == *"m.dat:4: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
}

# --- sanitizing and the entry point ----------------------------------------

@test "a path in an exempt tree is still replaced by an opaque identifier" {
	mkdir -p .devtools
	echo "zebra-host-01" >.devtools/10.1.2.3.txt
	git add .devtools
	run_check --staged
	[ "$status" -eq 1 ]
	[[ $output =~ path#[0-9a-f]{8}:1:\ private\ literal\ O11Y_PRIVATE_STRINGS\ entry\ 1 ]]
	[[ $output != *".txt"* ]]
}

@test "the mise task keeps argument boundaries" {
	command -v mise >/dev/null
	local line
	line=$(awk '/^\["devtools:lint-leaks"\]/ { f = 1 } f && /^run = / { print; exit }' "$repo_root/.devtools/tasks/leaks.toml")
	[[ $line == 'run = "bash $DEVTOOLS_DIR/scripts/check-leaks.sh"' ]]
	printf '[tasks."devtools:lint-leaks"]\nquiet = true\nrun = "bash %s"\n' "$checker" >mise.toml
	export DEVTOOLS_DIR="$work/.devtools"
	mise trust -q "$work/mise.toml"
	echo "fine" >"a file with spaces.md"
	run mise run devtools:lint-leaks -- "a file with spaces.md"
	[ "$status" -eq 0 ]
	echo "zebra-host-01" >"leak with spaces.md"
	run mise run devtools:lint-leaks -- "leak with spaces.md"
	[ "$status" -eq 1 ]
	[[ $output == *"private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
	no_secrets
}

# --- the ref lines ------------------------------------------------------------

zero=0000000000000000000000000000000000000000

# lines LINE...: feed ref-update lines to --pre-push on stdin, as git does.
lines() {
	# A destination that already has the starting commit, unless a case names
	# another one in $dest.
	if [[ ! -d $work/origin.git ]]; then
		git init -q --bare "$work/origin.git"
		git push -q "$work/origin.git" "$base:refs/heads/main"
	fi
	printf '%s\n' "$@" >"$BATS_TEST_TMPDIR/lines"
	run bash "$checker" --pre-push origin "${dest:-$work/origin.git}" <"$BATS_TEST_TMPDIR/lines"
	no_secrets
}

leaky_history() {
	echo "clean" >a.md
	git add a.md
	git commit -q -m "chore: clean"
	clean=$(git rev-parse HEAD)
	echo "host zebra-host-01" >b.md
	git add b.md
	git commit -q -m "chore: leaky"
	tip=$(git rev-parse HEAD)
}

@test "--pre-push scans what the line's remote oid doesn't reach" {
	leaky_history
	lines "refs/heads/f $tip refs/heads/f $zero"
	[ "$status" -eq 1 ]
	[[ $output == *"b.md:1: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
	lines "refs/heads/f $tip refs/heads/f $tip"
	[ "$status" -eq 0 ]
	lines "refs/heads/f $tip refs/heads/f $clean"
	[ "$status" -eq 1 ]
	lines "refs/heads/f $clean refs/heads/f $base"
	[ "$status" -eq 0 ]
}

@test "--pre-push excludes nothing for a remote oid that isn't here" {
	leaky_history
	lines "refs/heads/f $tip refs/heads/f 1234567890123456789012345678901234567890"
	[ "$status" -eq 1 ]
}

@test "--pre-push ignores remote-tracking refs and the environment" {
	leaky_history
	git update-ref refs/remotes/origin/main "$tip"
	PRE_COMMIT_REMOTE_NAME=origin PRE_COMMIT_TO_REF=$tip lines "refs/heads/f $tip refs/heads/f $zero"
	[ "$status" -eq 1 ]
}

@test "--pre-push scans every line and skips a deletion's objects" {
	leaky_history
	lines "(delete) $zero refs/heads/gone 1234567890123456789012345678901234567890"
	[ "$status" -eq 0 ]
	lines "refs/heads/ok $clean refs/heads/ok $base" "refs/heads/f $tip refs/heads/f $clean"
	[ "$status" -eq 1 ]
}

@test "--pre-push rejects a malformed line" {
	leaky_history
	lines "nonsense"
	[ "$status" -eq 2 ]
	lines "refs/heads/f nothex refs/heads/f $zero"
	[ "$status" -eq 2 ]
	lines "refs/heads/f $tip refs/heads/f $zero extra"
	[ "$status" -eq 2 ]
}

@test "--pre-push asks the destination what it has when a ref is new" {
	# The destination already holds a leaking commit; a clean branch off it passes.
	echo "host zebra-host-01" >old.md
	git add old.md
	git commit -q -m "chore: old leak"
	old=$(git rev-parse HEAD)
	git init -q --bare "$work/origin.git"
	git push -q "$work/origin.git" "$old:refs/heads/main"
	echo ok >new.md
	git add new.md
	git commit -q -m "chore: clean"
	tip=$(git rev-parse HEAD)
	lines "refs/heads/f $tip refs/heads/f $zero"
	[ "$status" -eq 0 ]
	echo "another host zebra-host-01" >new2.md
	git add new2.md
	git commit -q -m "chore: new leak"
	lines "refs/heads/f $(git rev-parse HEAD) refs/heads/f $zero"
	[ "$status" -eq 1 ]
	[[ $output == *"new2.md:1: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
}

@test "--pre-push scans everything for an empty destination, whatever the tracking refs say" {
	leaky_history
	git update-ref refs/remotes/origin/main "$tip"
	git init -q --bare "$work/empty.git"
	dest="$work/empty.git" lines "refs/heads/f $tip refs/heads/f $zero"
	[ "$status" -eq 1 ]
	[[ $output == *"b.md:1: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
}

@test "--pre-push fails when the destination can't be asked" {
	leaky_history
	dest="$work/nowhere.git" lines "refs/heads/f $tip refs/heads/f $zero"
	[ "$status" -eq 2 ]
	[[ $output == *"incomplete"* ]]
}

@test "--pre-push without a URL excludes nothing for a new ref" {
	leaky_history
	printf '%s\n' "refs/heads/f $tip refs/heads/f $zero" >"$BATS_TEST_TMPDIR/lines"
	run_check --pre-push origin <"$BATS_TEST_TMPDIR/lines"
	[ "$status" -eq 1 ]
}

@test "--pre-push scans ref names" {
	echo ok >a.md
	git add a.md
	git commit -q -m "chore: ok"
	lines "refs/heads/zebra-host-01-fix $(git rev-parse HEAD) refs/heads/zebra-host-01-fix $zero"
	[ "$status" -eq 1 ]
	[[ $output =~ path#[0-9a-f]{8}:path:\ private\ literal\ O11Y_PRIVATE_STRINGS\ entry\ 1 ]]
}

@test "--pre-push with no lines scans everything reachable from HEAD" {
	leaky_history
	run_check --pre-push </dev/null
	[ "$status" -eq 1 ]
}

@test "--pre-push and --range refuse a shallow repository" {
	leaky_history
	git clone -q --depth 1 "file://$work" "$BATS_TEST_TMPDIR/shallow"
	cd "$BATS_TEST_TMPDIR/shallow"
	run_check --pre-push </dev/null
	[ "$status" -eq 2 ]
	run_check --range HEAD
	[ "$status" -eq 2 ]
}

@test "a missing object fails the scan" {
	echo "unique content for a loose blob" >c.md
	git add c.md
	git commit -q -m "chore: c"
	oid=$(git rev-parse HEAD:c.md)
	rm "$(git rev-parse --git-path "objects/${oid:0:2}/${oid:2}")"
	run_check --range "$base..HEAD"
	[ "$status" -eq 2 ]
	[[ $output == *"incomplete"* ]]
}

@test "an annotated tag, nested or not, is scanned with its target" {
	echo ok >a.md
	git add a.md
	git commit -q -m "chore: ok"
	git tag -a -m "outer zebra-host-01" t1
	git tag -a -m "nested" t2 t1 2>/dev/null
	lines "refs/tags/t2 $(git rev-parse t2) refs/tags/t2 $zero"
	[ "$status" -eq 1 ]
	[[ $output == *"tag "*"private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
}

# --- the installed hook, with real pushes --------------------------------------

# hook_repo: this repository's hook wrapper installed in front of prek's, with
# bare repositories to push to.
hook_repo() {
	command -v prek >/dev/null
	command -v mise >/dev/null
	mkdir -p .devtools/scripts
	cp "$checker" .devtools/scripts/check-leaks.sh
	cp "$BATS_TEST_DIRNAME/install-pre-push.sh" .devtools/scripts/install-pre-push.sh
	# The real task definition, so the hook runs the entry the repository ships.
	awk '/^\["devtools:lint-leaks"\]/ { f = 1 } f && /^$/ { exit } f { print }' "$repo_root/.devtools/tasks/leaks.toml" | sed 's/^\["devtools:/[tasks."devtools:/' >mise.toml
	[[ -s mise.toml ]]
	export DEVTOOLS_DIR="$work/.devtools"
	mise trust -q "$work/mise.toml"
	cat >.pre-commit-config.yaml <<YAML
---
repos:
  - repo: local
    hooks:
      - id: saw
        name: saw
        entry: touch $BATS_TEST_TMPDIR/prek-ran
        language: system
        stages: [pre-push]
        always_run: true
        pass_filenames: false
YAML
	git ls-files --others --exclude-standard -z | xargs -0 git add --
	git commit -q -m "chore: hook"
	git init -q --bare "$work/A.git"
	git init -q --bare "$work/B.git"
	git remote add origin "$work/A.git"
	git push -q origin main
	git config --unset core.hooksPath
	prek install -t pre-push >/dev/null 2>&1
	bash .devtools/scripts/install-pre-push.sh >/dev/null
	rm -f "$BATS_TEST_TMPDIR/prek-ran"
}

push_ok() {
	run git push "$@"
	no_secrets
	[ "$status" -eq 0 ]
}

push_blocked() {
	run git push "$@"
	[ "$status" -ne 0 ]
	# Only the check's own lines are ours; git prints ref names itself.
	# A negated command that isn't the last line asserts nothing in bats.
	if grep -E 'check-leaks|private literal|pattern:' <<<"$output" | grep -qF -e zebra-host-01 -e kestrel99; then
		return 1
	fi
}

@test "a clean push passes through the wrapper to prek's hook" {
	hook_repo
	git switch -q -c clean
	echo ok >c.md
	git add c.md
	git commit -q -m "chore: clean"
	push_ok origin clean
	[ -f "$BATS_TEST_TMPDIR/prek-ran" ]
}

@test "the hook blocks a push of two refs when only one leaks, in either order" {
	hook_repo
	git switch -q -c clean
	echo ok >c.md
	git add c.md
	git commit -q -m "chore: clean"
	git switch -q -c leaky main
	echo "host zebra-host-01" >l.md
	git add l.md
	git commit -q -m "chore: leaky"
	push_blocked origin clean leaky
	[[ $output == *"private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
	push_blocked origin leaky clean
	[ -z "$(git ls-remote origin refs/heads/clean refs/heads/leaky)" ]
	[ ! -f "$BATS_TEST_TMPDIR/prek-ran" ]
}

@test "the hook scans a tag-only push of an annotated tag" {
	hook_repo
	git tag -a -m "release for zebra-host-01" v1
	push_blocked origin v1
	[[ $output == *"private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
	[ -z "$(git ls-remote origin refs/tags/v1)" ]
	git tag -d v1 >/dev/null
	git tag -a -m "clean release" v1
	push_ok origin v1
}

@test "the hook scans a new branch whose history holds a leak the tip removed" {
	hook_repo
	git switch -q -c work
	echo "host zebra-host-01" >l.md
	git add l.md
	git commit -q -m "chore: add"
	git rm -q l.md
	git commit -q -m "chore: remove"
	push_blocked origin work
	[[ $output == *"l.md:1: private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
}

@test "the hook trusts nothing about an origin that was repointed to an empty repository" {
	hook_repo
	git switch -q -c work
	echo "host zebra-host-01" >l.md
	git add l.md
	git commit -q -m "chore: leak"
	git push -q --no-verify origin work
	echo ok >c.md
	git add c.md
	git commit -q -m "chore: clean child"
	git remote set-url origin "$work/B.git"
	push_blocked origin work
	[ -z "$(git ls-remote "$work/B.git")" ]
}

@test "the hook covers every push URL of a remote" {
	hook_repo
	git switch -q -c work
	echo "host zebra-host-01" >l.md
	git add l.md
	git commit -q -m "chore: leak"
	git push -q --no-verify origin work
	echo ok >c.md
	git add c.md
	git commit -q -m "chore: clean child"
	git remote set-url --push origin "$work/A.git"
	git remote set-url --add --push origin "$work/B.git"
	push_blocked origin work
	[ -z "$(git ls-remote "$work/B.git")" ]
}

@test "the hook scans author and committer lines" {
	hook_repo
	git switch -q -c work
	echo ok >c.md
	git add c.md
	GIT_AUTHOR_NAME=zebra-host-01 git commit -q -m "chore: authored"
	push_blocked origin work
	[[ $output == *"commit "*"private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
	git switch -q -c work2 main
	echo ok2 >d.md
	git add d.md
	GIT_COMMITTER_EMAIL=me@zebra-host-01.example git commit -q -m "chore: committed"
	push_blocked origin work2
	[[ $output == *"commit "*"private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
}

@test "the hook scans a branch name" {
	hook_repo
	git switch -q -c zebra-host-01-fix
	echo ok >c.md
	git add c.md
	git commit -q -m "chore: ok"
	push_blocked origin zebra-host-01-fix
	[[ $output =~ path#[0-9a-f]{8}:path:\ private\ literal\ O11Y_PRIVATE_STRINGS\ entry\ 1 ]]
}

@test "the hook blocks a push when an object is missing" {
	hook_repo
	git switch -q -c work
	echo "a loose blob that is unique" >c.md
	git add c.md
	git commit -q -m "chore: c"
	oid=$(git rev-parse HEAD:c.md)
	rm "$(git rev-parse --git-path "objects/${oid:0:2}/${oid:2}")"
	push_blocked origin work
	[[ $output == *"incomplete"* ]]
}

@test "a new branch off a destination whose history holds a leak passes when its own commits are clean" {
	hook_repo
	git switch -q -c old
	echo "host zebra-host-01" >l.md
	git add l.md
	git commit -q -m "chore: old leak"
	git push -q --no-verify origin old:main
	git switch -q -c fresh
	echo ok >c.md
	git add c.md
	git commit -q -m "chore: clean"
	push_ok origin fresh
	echo "second host zebra-host-01" >l2.md
	git add l2.md
	git commit -q -m "chore: new leak"
	push_blocked origin fresh:fresh2
}

@test "the wrapper skips the check, with a warning, on a branch that predates it" {
	hook_repo
	git switch -q -c old
	rm .devtools/scripts/check-leaks.sh
	echo "host zebra-host-01" >l.md
	git ls-files --others --exclude-standard -z | xargs -0 git add --
	git commit -q -m "chore: no checker here"
	push_ok origin old
	[[ $output == *"leak check is not present on this branch and was skipped"* ]]
	[ -f "$BATS_TEST_TMPDIR/prek-ran" ]
}

@test "deleting a remote branch passes" {
	hook_repo
	git push -q origin main:old
	push_ok origin :old
}

@test "the check doesn't echo the arguments mise is given" {
	command -v mise >/dev/null
	awk '/^\["devtools:lint-leaks"\]/ { f = 1 } f && /^$/ { exit } f { print }' "$repo_root/.devtools/tasks/leaks.toml" | sed 's/^\["devtools:/[tasks."devtools:/' >mise.toml
	sed -i.bak "s|\$DEVTOOLS_DIR/scripts/check-leaks.sh|$checker|" mise.toml
	export DEVTOOLS_DIR="$repo_root/.devtools"
	mise trust -q "$work/mise.toml"
	echo ok >zebra-host-01.txt
	run mise run devtools:lint-leaks -- zebra-host-01.txt
	no_secrets
	[ "$status" -eq 1 ]
}

# --- the installer ---------------------------------------------------------------

@test "the installer puts the wrapper in front of prek's hook, and reinstalling keeps it" {
	hook_repo
	hooks=$(git rev-parse --git-path hooks)
	grep -qF 'check-leaks pre-push wrapper' "$hooks/pre-push"
	grep -qF 'File generated by prek' "$hooks/pre-push.prek"
	bash .devtools/scripts/install-pre-push.sh --check
	bash .devtools/scripts/install-pre-push.sh >/dev/null
	bash .devtools/scripts/install-pre-push.sh --check
	# prek install parks the wrapper as legacy and writes its own hook.
	prek install -t pre-push >/dev/null 2>&1
	run bash .devtools/scripts/install-pre-push.sh --check
	[ "$status" -eq 1 ]
	bash .devtools/scripts/install-pre-push.sh >/dev/null
	bash .devtools/scripts/install-pre-push.sh --check
	[ ! -e "$hooks/pre-push.legacy" ]
}

@test "the installer check fails without the wrapper, and is skipped in CI" {
	hook_repo
	hooks=$(git rev-parse --git-path hooks)
	rm "$hooks/pre-push"
	run bash .devtools/scripts/install-pre-push.sh --check
	[ "$status" -eq 1 ]
	CI=true run bash .devtools/scripts/install-pre-push.sh --check
	[ "$status" -eq 0 ]
}

@test "the installer check fails when reading the hook prints it and then fails" {
	hook_repo
	hooks=$(git rev-parse --git-path hooks)
	bash .devtools/scripts/install-pre-push.sh --check
	real=$(command -v cat)
	mkdir "$BATS_TEST_TMPDIR/bin"
	printf '#!/bin/sh\ncase "$1" in */pre-push) %s "$1"; exit 1 ;; esac\nexec %s "$@"\n' "$real" "$real" >"$BATS_TEST_TMPDIR/bin/cat"
	chmod +x "$BATS_TEST_TMPDIR/bin/cat"
	PATH="$BATS_TEST_TMPDIR/bin:$PATH" run bash .devtools/scripts/install-pre-push.sh --check
	[ "$status" -eq 1 ]
}

@test "the installer refuses to wrap a hook it doesn't know" {
	hook_repo
	hooks=$(git rev-parse --git-path hooks)
	printf '#!/bin/sh\nexit 0\n' >"$hooks/pre-push"
	run bash .devtools/scripts/install-pre-push.sh
	[ "$status" -eq 1 ]
}

@test "the wrapper works from a linked worktree" {
	hook_repo
	git worktree add -q -b linked "$BATS_TEST_TMPDIR/linked" main
	cd "$BATS_TEST_TMPDIR/linked"
	bash .devtools/scripts/install-pre-push.sh --check
	echo "host zebra-host-01" >l.md
	git add l.md
	git commit -q -m "chore: leak"
	push_blocked origin linked
	[[ $output == *"private literal O11Y_PRIVATE_STRINGS entry 1"* ]]
}

# --- round four: exemptions per path, failures that read as success ----------

@test "a blob reused under an exempt path and a plain one is pattern-scanned" {
	mkdir -p .agents
	echo "connect 10.21.32.43" >.agents/example.txt
	cp .agents/example.txt deployment.txt
	git ls-files --others --exclude-standard -z | xargs -0 git add --
	git commit -q -m "chore: reuse"
	run_check --range "$base..HEAD"
	[ "$status" -eq 1 ]
	[[ $output == *"deployment.txt:1: pattern: private IPv4 address"* ]]
}

@test "a blob used only under exempt paths stays exempt from patterns" {
	mkdir -p .agents .devtools
	echo "connect 10.21.32.43" >.agents/example.txt
	cp .agents/example.txt .devtools/example.txt
	git ls-files --others --exclude-standard -z | xargs -0 git add --
	git commit -q -m "chore: exempt only"
	run_check --range "$base..HEAD"
	[ "$status" -eq 0 ]
}

@test "a reused blob is caught through the installed hook as well" {
	hook_repo
	mkdir -p .agents
	git switch -q -c work
	echo "connect 10.21.32.43" >.agents/example.txt
	cp .agents/example.txt deployment.txt
	git ls-files --others --exclude-standard -z | xargs -0 git add --
	git commit -q -m "chore: reuse"
	push_blocked origin work
	[[ $output == *"pattern: private IPv4 address"* ]]
}

@test "a gitlink path is scanned though git doesn't list it" {
	git update-index --add --cacheinfo "160000,$base,10.1.2.3"
	git commit -q -m "chore: gitlink"
	run_check --range "$base..HEAD"
	[ "$status" -eq 1 ]
	[[ $output =~ path#[0-9a-f]{8}:path:\ pattern:\ private\ IPv4\ address ]]
}

@test "content that looks like a cat-file answer is only content" {
	printf 'something missing\n' >plain.txt
	git add plain.txt
	git commit -q -m "chore: odd text"
	run_check --range "$base..HEAD"
	[ "$status" -eq 0 ]
}

@test "--pre-push-hook with no ref lines has nothing to push, and --pre-push scans HEAD" {
	leaky_history
	run_check --pre-push-hook origin "$work/origin.git" </dev/null
	[ "$status" -eq 0 ]
	run_check --pre-push </dev/null
	[ "$status" -eq 1 ]
}

# fake NAME BODY: put a stand-in for a command first on PATH.
fake() {
	mkdir -p "$BATS_TEST_TMPDIR/shim"
	printf '#!/bin/sh\n%s\n' "$2" >"$BATS_TEST_TMPDIR/shim/$1"
	chmod +x "$BATS_TEST_TMPDIR/shim/$1"
	PATH="$BATS_TEST_TMPDIR/shim:$PATH"
}

@test "the wrapper blocks a push when buffering git's input fails" {
	hook_repo
	git switch -q -c work
	echo "host zebra-host-01" >l.md
	git add l.md
	git commit -q -m "chore: leak"
	# A cat that eats its input and reports failure, so nothing reaches the check.
	real_cat=$(command -v cat)
	fake cat "$real_cat >/dev/null; exit 1"
	push_blocked origin work
	[[ $output == *"could not read the push from git"* ]]
	[ -z "$(git ls-remote "$work/A.git" refs/heads/work)" ]
}

@test "an unreadable private-literals file fails the scan instead of passing it" {
	real=$(command -v awk)
	fake awk "[ -n \"\$NEEDLES\" ] && rm -f \"\$NEEDLES\"; exec $real \"\$@\""
	echo "connect 10.21.32.43" >zebra-host-01.txt
	run_check zebra-host-01.txt
	[ "$status" -eq 2 ]
	[[ $output == *"incomplete"* ]]
	# Nothing from the partial scan may reach the output: no finding, no name.
	[[ $output != *"pattern:"* ]]
	[[ $output != *"zebra-host-01"* ]]
	[[ $output != *"10.21.32.43"* ]]
}

@test "an unreadable path list fails an object scan instead of passing it" {
	real=$(command -v awk)
	fake awk "[ -n \"\$PAIRS\" ] && rm -f \"\$PAIRS\"; exec $real \"\$@\""
	echo ok >c.md
	git add c.md
	git commit -q -m "chore: c"
	run_check --range "$base..HEAD"
	[ "$status" -ne 0 ]
}

@test "the installer won't overwrite an unrelated pre-push.prek" {
	hook_repo
	hooks=$(git rev-parse --git-path hooks)
	prek install -t pre-push >/dev/null 2>&1
	mv "$hooks/pre-push.legacy" "$BATS_TEST_TMPDIR/legacy" 2>/dev/null || true
	printf 'unrelated\n' >"$hooks/pre-push.prek"
	run bash .devtools/scripts/install-pre-push.sh
	[ "$status" -eq 1 ]
	[ "$(cat "$hooks/pre-push.prek")" = unrelated ]
}

@test "private key headers are findings without their text" {
	for kind in '' 'RSA ' 'EC ' 'OPENSSH ' 'ENCRYPTED '; do
		printf '%s\n' "-----BEGIN ${kind}PRIVATE KEY-----" >key.pem
		run_check key.pem
		[ "$status" -eq 1 ]
		[[ $output == *"pattern: private-key header"* ]]
		[[ $output != *"BEGIN"* ]]
	done
}

@test "repository rules extend patterns and never print the rule or match" {
	echo 'custom-[a-z]+' >.leak-patterns
	echo 'custom-secret' >sample.txt
	run_check sample.txt
	[ "$status" -eq 1 ]
	[[ $output == *"repository rule 1"* ]]
	[[ $output != *"custom-"* ]]
}

@test "any exported private-strings variable is loaded" {
	export PROJECT_PRIVATE_STRINGS='special-literal, another-literal'
	echo SPECIAL-LITERAL >sample.txt
	run_check sample.txt
	[ "$status" -eq 1 ]
	[[ $output == *"PROJECT_PRIVATE_STRINGS entry 1"* ]]
	[[ $output != *"SPECIAL-LITERAL"* ]]
}

@test "invalid repository patterns fail without echoing the expression" {
	echo '[secret-pattern' >.leak-patterns
	echo ok >sample.txt
	run_check sample.txt
	[ "$status" -eq 2 ]
	[[ $output != *"secret-pattern"* ]]
}

@test "invalid allow expressions fail without echoing their contents" {
	echo '[sensitive-allow' >.leak-allow
	echo ok >sample.txt
	run_check sample.txt
	[ "$status" -eq 2 ]
	[[ $output != *"sensitive-allow"* ]]
}
