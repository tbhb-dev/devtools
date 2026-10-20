#!/usr/bin/env bash
# gomodscan v0.1.0 logs lookup failures but returns a clean/findings status.
# Accept only its complete-run summary on stderr; preserve every diagnostic.
set -euo pipefail
format=$1
tracked=$(git ls-files -- vendor/modules.txt)
if [ -z "$tracked" ]; then
  if [ "$format" = text ]; then
    echo "gomodscan: skipping module without tracked vendor/modules.txt"
    exit 0
  fi
  echo "gomodscan: SARIF requires tracked vendor/modules.txt" >&2
  exit 2
fi
errors=$(mktemp)
trap 'rm -f "$errors"' EXIT
if [ "$format" = sarif ]; then
  # Open separately: a redirection failure must never become a findings exit.
  if ! exec 3>"$2"; then
    echo "gomodscan: cannot write SARIF output: $2" >&2
    exit 2
  fi
else
  exec 3>&1
fi
rc=0
if [ "$format" = sarif ]; then
  go tool gomodscan -format sarif >&3 2>"$errors" || rc=$?
else
  go tool gomodscan >&3 2>"$errors" || rc=$?
fi
exec 3>&-
cat "$errors" >&2
if [ "$rc" -gt 1 ]; then
  echo "gomodscan: scan could not run; check network access to the scanner services and retry." >&2
  exit "$rc"
fi
# A missing summary or any extra line fails closed, including unknown diagnostics
# after a tool upgrade. Findings themselves are written to stdout, not stderr.
if [ "$rc" -eq 0 ]; then
  summary='gomodscan: scanned [0-9]+ module\(s\), no findings'
else
  summary='gomodscan: [0-9]+ finding\(s\) across [0-9]+ module\(s\)'
fi
lines=$(wc -l <"$errors")
if [ "$lines" -ne 1 ] || ! grep -Eqx "$summary" "$errors" ||
  grep -E -v -q -x "$summary" "$errors"; then
  echo "gomodscan: incomplete scan; unexpected or missing completion diagnostic${2:+; output: $2}" >&2
  exit 2
fi
if [ "$format" = sarif ]; then
  if [ ! -f "$2" ] || ! jq -se 'length == 1 and (.[0] | type == "object" and .version == "2.1.0" and
    (.runs | type == "array" and length > 0) and
    all(.runs[]; (.tool.driver.name | type == "string" and length > 0) and
      (.results | type == "array") and
      all(.results[]; (.message.text | type == "string"))))' "$2" >/dev/null; then
    echo "gomodscan: invalid SARIF output: $2" >&2
    exit 2
  fi
  exit 0
fi
exit "$rc"
