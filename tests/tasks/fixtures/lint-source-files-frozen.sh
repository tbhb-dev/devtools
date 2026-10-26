#!/usr/bin/env bash
set -euo pipefail
mode=$1
shift
excludes=(':(exclude)vendor/**' ':(exclude)apm_modules/**')
while IFS= read -r path; do
  if [[ -n "$path" ]]; then
    path=${path%/}
    excludes+=(":(exclude)$path" ":(exclude)$path/**")
  fi
done <<<"${DEVTOOLS_GENERATED_PATHS:-}"
case "$mode" in
shell)
  pattern='*.sh'
  command=(shellcheck -o check-extra-masked-returns)
  ;;
curl)
  pattern='*.sh'
  command=(uv run --script "$DEVTOOLS_DIR/scripts/check_curl_fail.py")
  ;;
shell-fmt)
  pattern='*.sh'
  command=(shfmt -d)
  ;;
prose)
  pattern='*'
  command=(vale --output=ai-tells-agent.tmpl --glob="${DEVTOOLS_PROSE_GLOB:-*}")
  ;;
markdown-wrap)
  pattern='*.md'
  command=(guard-markdown)
  ;;
esac
selected=$(mktemp)
trap 'rm -f "$selected"' EXIT
patterns=("$pattern")
if [[ "$mode" == prose && $# -gt 0 ]]; then
  patterns=("$@")
  shift "$#"
fi
git ls-files -z --cached --others --exclude-standard -- "${patterns[@]}" "${excludes[@]}" >"$selected"
uv run --script "$DEVTOOLS_DIR/scripts/filter-apm-deployed.py" "$selected"
files=()
while IFS= read -r -d '' file; do
  files+=("$file")
done <"$selected"
if [[ -n ${files[0]+set} ]]; then
  "${command[@]}" "$@" "${files[@]}"
fi
