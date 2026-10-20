#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  REAL_GO=$(command -v go)
  ROOT=$(cd "$BATS_TEST_DIRNAME/../.." && pwd)
  mkdir -p "$BATS_TEST_TMPDIR/consumer/bin"
  cd "$BATS_TEST_TMPDIR/consumer"
  git init -q
  mkdir -p vendor
  printf 'module example.com/consumer\n' >go.mod
  printf '# example.com/dependency v1.0.0\n' >vendor/modules.txt
  git add -- go.mod vendor/modules.txt
  cat >bin/go <<'STUB'
#!/usr/bin/env bash
if [ "${3:-}" = -format ]; then
  case "$SCAN_OUTPUT" in
    valid) printf '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gomodscan"}},"results":[]}]}' ;;
    invalid) printf '{}' ;;
    multiple) printf '%s\n' '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gomodscan"}},"results":[]}]}' '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gomodscan"}},"results":[]}]}' ;;
    empty) : ;;
    missing) rm "$usage_file" ;;
  esac
fi
case "$SCAN_RESULT" in
  clean) echo 'gomodscan: scanned 1 module(s), no findings' >&2 ;;
  findings) echo 'gomodscan: 1 finding(s) across 1 module(s)' >&2; exit 1 ;;
  lookup)
    echo 'gomodscan: example.com/dependency: lookup versions: unexpected status from pkg.go.dev: 500' >&2
    echo 'gomodscan: scanned 1 module(s), no findings' >&2 ;;
  lookup-findings)
    echo 'gomodscan: example.com/dependency: lookup vulns: unexpected status from osv.dev: 500' >&2
    echo 'gomodscan: 1 finding(s) across 1 module(s)' >&2; exit 1 ;;
  unknown) echo 'new scanner diagnostic' >&2 ;;
  unterminated)
    echo 'gomodscan: scanned 1 module(s), no findings' >&2
    printf 'lookup failure without newline' >&2 ;;
  silent) : ;;
  failure) echo 'connection refused' >&2; exit 2 ;;
esac
STUB
  chmod +x bin/go
  export PATH="$PWD/bin:$PATH" DEVTOOLS_DIR="$ROOT/.devtools"
  export SCAN_RESULT=clean SCAN_OUTPUT=valid usage_file=scan.sarif
}

scan() {
  # Execute the actual task entry point, including its output policy.
  local task=$1
  local command
  command=$(
    python3 - "$ROOT/.devtools/tasks/go.toml" "$task" <<'PY'
import sys
import tomllib
with open(sys.argv[1], "rb") as source:
    print(tomllib.load(source)[sys.argv[2]]["run"])
PY
  )
  bash -c "$command"
}

@test "text scan skips a module without tracked vendor/modules.txt" {
  git rm -q -f -- vendor/modules.txt
  export SCAN_RESULT=failure
  run scan devtools:gomodscan
  [ "$status" -eq 0 ]
  [[ "$output" == *'skipping module without tracked vendor/modules.txt'* ]]
  [ ! -d vendor ]
  mkdir vendor
  echo untracked >vendor/modules.txt
  run scan devtools:gomodscan
  [ "$status" -eq 0 ]
  [[ "$output" == *'skipping module'* ]]
}

@test "SARIF requires a vendored module" {
  git rm -q -f -- vendor/modules.txt
  run scan devtools:gomodscan-sarif
  [ "$status" -gt 1 ]
  [ ! -e "$usage_file" ]
}

@test "complete clean scans and findings retain the text and SARIF policies" {
  local result expected
  for result in clean findings; do
    export SCAN_RESULT=$result
    expected=0
    if [ "$result" = findings ]; then expected=1; fi
    run scan devtools:gomodscan
    [ "$status" -eq "$expected" ]
    run scan devtools:gomodscan-sarif
    [ "$status" -eq 0 ]
    jq -e '.version == "2.1.0"' "$usage_file"
  done
}

@test "both modes reject lookup errors even with a clean or findings summary" {
  local result task
  for result in lookup lookup-findings unknown unterminated silent failure; do
    export SCAN_RESULT=$result
    for task in devtools:gomodscan devtools:gomodscan-sarif; do
      run scan "$task"
      [ "$status" -gt 1 ]
      if [[ "$result" == lookup* ]]; then
        [[ "$output" == *'example.com/dependency: lookup'* ]]
        [[ "$output" == *'500'* ]]
      fi
    done
  done
}

@test "SARIF output failures never become successful findings" {
  local result path
  for result in clean findings; do
    export SCAN_RESULT=$result
    for path in missing/scan.sarif . /dev/full; do
      export usage_file=$path
      run scan devtools:gomodscan-sarif
      [ "$status" -gt 1 ]
      [[ "$output" == *"$path"* || "$output" == *'scan could not run'* ]]
    done
  done
}

@test "SARIF success requires an existing nonempty report with SARIF structure" {
  local payload result
  for result in clean findings; do
    export SCAN_RESULT=$result
    for payload in invalid empty missing multiple; do
      export SCAN_OUTPUT=$payload
      run scan devtools:gomodscan-sarif
      [ "$status" -gt 1 ]
      [[ "$output" == *'invalid SARIF output: scan.sarif'* ]]
    done
  done
}

@test "pinned scanner HTTP 500 fails closed in both task modes" {
  # Redirect only the HTTP endpoints in a scratch copy. Exercise real lookup,
  # logging, exit, and report emission code without modifying the dependency.
  local scanner_source
  scanner_source=$(cd "$ROOT" && "$REAL_GO" list -mod=mod -f '{{.Dir}}' github.com/proofhouse/gomodscan/cmd/gomodscan)
  cp -R "$scanner_source/../.." "$BATS_TEST_TMPDIR/scanner-source"
  chmod -R u+w "$BATS_TEST_TMPDIR/scanner-source"
  python3 - "$scanner_source/main.go" "$BATS_TEST_TMPDIR" <<'PY'
import pathlib
import sys

original = pathlib.Path(sys.argv[1])
work = pathlib.Path(sys.argv[2]) / "scanner-source"
replacement = work / "cmd/gomodscan/main.go"
replacement.chmod(0o644)
source = original.read_text()
source = source.replace('"io"', '"io"\n"net/http"\n"net/http/httptest"')
source = source.replace('&pkgsite.Client{}', '&pkgsite.Client{BaseURL: os.Getenv("SCAN_SERVER")}')
source = source.replace('&osv.Client{}', '&osv.Client{BaseURL: os.Getenv("SCAN_SERVER")}')
source += '''
func init() {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if strings.Contains(r.URL.Path, "/versions/") {
            w.WriteHeader(http.StatusInternalServerError)
            return
        }
        fmt.Fprint(w, `{}`)
    }))
    os.Setenv("SCAN_SERVER", server.URL)
}
'''
replacement.write_text(source)
PY
  (cd "$BATS_TEST_TMPDIR/scanner-source" && GOWORK=off "$REAL_GO" build -buildvcs=false -mod=readonly \
    -o "$BATS_TEST_TMPDIR/scanner" ./cmd/gomodscan)
  printf '#!/usr/bin/env bash\nshift 2\nexec "%s/scanner" "$@"\n' "$BATS_TEST_TMPDIR" >bin/go
  for task in devtools:gomodscan devtools:gomodscan-sarif; do
    run scan "$task"
    [ "$status" -eq 2 ]
    [[ "$output" == *'example.com/dependency: lookup versions:'* ]]
    [[ "$output" == *'unexpected status from pkg.go.dev'* ]]
    [[ "$output" == *'500'* ]]
  done
}
