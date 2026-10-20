#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
	ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
	WORK="${BATS_TEST_TMPDIR}/consumer"
	mkdir -p "$WORK/.devtools" "$WORK/.config/mise/conf.d"
	cp -R "$ROOT/.devtools/tasks" "$ROOT/.devtools/scripts" "$ROOT/.devtools/config" "$WORK/.devtools/"
	cp "$ROOT/.config/mise/conf.d/10-devtools-tools.toml" "$ROOT/.config/mise/conf.d/20-devtools-env.toml" "$WORK/.config/mise/conf.d/"
	cp "$ROOT/.config/mise/mise.lock" "$WORK/.config/mise/"
	cat >"$WORK/mise.toml" <<'TOML'
[task_config]
includes = [".devtools/tasks/go.toml"]
TOML
	cp "$ROOT/.devtools/vendir.yml" "$WORK/vendir.yml"
	cd "$WORK"
	export MISE_TRUSTED_CONFIG_PATHS="$WORK"
	export MISE_LOCKED=1
	export DEVTOOLS_DIR="$WORK/.devtools"
	unset DEVTOOLS_GO_PACKAGES DEVTOOLS_GO_MODULE DEVTOOLS_GO_BIN_PACKAGES
	unset DEVTOOLS_GO_MODULES_MODE DEVTOOLS_PYTHON_SOURCE_DIRS DEVTOOLS_PYTHON_DIRS
}

@test "scratch Go consumer runs shipped lint and test tasks and rejects defects" {
	printf 'module example.com/consumer\n\ngo 1.27.1\n' >go.mod
	sed -e 's|example.com/project|example.com/consumer|g' \
		-e 's/Copyright YOUR AUTHORS/Copyright Tony Burns/' \
		.devtools/config/go/.golangci.yml >.golangci.yml
	cat >double.go <<'GO'
// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumer

func Double(value int) int { return value + value }
GO
	cat >double_test.go <<'GO'
// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumer_test

import (
	"testing"

	"example.com/consumer"
)

func TestDouble(t *testing.T) {
	t.Parallel()
	if consumer.Double(1) != 2 {
		t.Fatal("wrong result")
	}
}
GO
	run mise run devtools:lint-go
	[ "$status" -eq 0 ] || {
		echo "$output"
		return 1
	}
	run mise run devtools:test-go
	[ "$status" -eq 0 ]
	# Exercise the actual publisher wrapper, including argument forwarding.
	awk '/^\[tasks.test-go\]$/ { selected = 1; print; next }
    selected && /^\[/ { exit }
    selected { print }' "$ROOT/mise.toml" >> mise.toml
	run mise run test-go -- -run '^$'
	[ "$status" -eq 0 ]
	[[ "$output" == *'no tests to run'* ]]
	sed 's/value + value/value/' double.go > changed
	mv changed double.go
	run mise run devtools:test-go
	[ "$status" -ne 0 ]
	[[ "$output" == *'wrong result'* ]]
	printf '\nfunc broken( {\n' >>double.go
	run mise run devtools:lint-go
	[ "$status" -ne 0 ]
}

@test "mutation config bounds Gremlins to a four-CPU runner" {
	for config in "$ROOT/.gremlins.yaml" "$ROOT/.devtools/config/go/.gremlins.yaml"; do
		run python3 - "$config" <<'PY'
import sys

import yaml

with open(sys.argv[1], encoding="utf-8") as stream:
    unleash = yaml.safe_load(stream)["unleash"]

assert unleash["workers"] == 2
assert unleash["test-cpu"] == 2
PY
		[ "$status" -eq 0 ] || {
			echo "$output"
			return 1
		}
	done
}

@test "ldflags derives the consumer module and honors metadata overrides" {
	printf 'module example.com/consumer\n\ngo 1.27.1\n' >go.mod
	run bash .devtools/scripts/go-ldflags.sh
	[ "$status" -eq 0 ]
	[[ "$output" == *'example.com/consumer/internal/buildmeta.Version=DEV'* ]]
	run env DEVTOOLS_GO_BUILDMETA=example.com/other/meta bash .devtools/scripts/go-ldflags.sh
	[ "$status" -eq 0 ]
	[[ "$output" == *'example.com/other/meta.Version=DEV'* ]]
}

@test "Python coverage expands consumer source roots rather than publisher paths" {
	sed 's|go.toml|python.toml|' mise.toml > changed
	mv changed mise.toml
	mkdir -p bin lib/one lib/two
	cat >bin/uv <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$@"
SH
	chmod +x bin/uv
	# Put the recorder ahead of mise's pinned uv when the task resolves PATH.
	cat >>mise.toml <<'TOML'
[env]
_.path = ["bin"]
TOML
	run env PATH="$WORK/bin:$PATH" DEVTOOLS_PYTHON_SOURCE_DIRS='lib/*' mise run devtools:cover-py
	[ "$status" -eq 0 ]
	[[ "$output" == *'--cov=lib/one'* ]]
	[[ "$output" == *'--cov=lib/two'* ]]
	[[ "$output" != *'packages/'* ]]
}

@test "vendored drift gate includes newly shipped scripts and templates" {
	git init -q
	git add -- .devtools .config mise.toml vendir.yml
	mkdir bin
	printf '#!/usr/bin/env bash\nexit 0\n' >bin/vendir
	chmod +x bin/vendir
	sed -n "/^run = '''$/,/^'''$/p" .devtools/tasks/vendir.toml | sed '1d;$d' >check.sh
	run env PATH="$WORK/bin:$PATH" bash check.sh
	[ "$status" -eq 0 ]
	echo '# drift' >>.devtools/scripts/go-ldflags.sh
	run env PATH="$WORK/bin:$PATH" bash check.sh
	[ "$status" -eq 1 ]
	[[ "$output" == *'go-ldflags.sh'* ]]
	git add -- .devtools/scripts/go-ldflags.sh
	echo '# new' >.devtools/config/new.yml
	run env PATH="$WORK/bin:$PATH" bash check.sh
	[ "$status" -eq 1 ]
	[[ "$output" == *'.devtools/config/new.yml'* ]]
}
