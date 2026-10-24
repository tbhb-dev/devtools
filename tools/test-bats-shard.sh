#!/usr/bin/env bash
# Validate the complete partition before running any shard. The sorted lists
# retain duplicates, so diff catches both missing and repeated suite files.
set -euo pipefail
export LC_ALL=C
unset CDPATH GREP_OPTIONS
IFS=$' \t\n'

root=$(git rev-parse --show-toplevel)
cd "$root"
manifest=tests/bats-shards.tsv
shard=${1:-}
case "$shard" in
1 | 2 | 3 | 4 | --check) ;;
*)
  printf 'usage: test-bats-shard.sh <1|2|3|4|--check>\n' >&2
  exit 1
  ;;
esac

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
find tests -type f -name '*.bats' | sort >"$work/expected"
awk '
  /^#/ || NF == 0 { next }
  NF != 3 || $1 !~ /^[1-4]$/ || $2 !~ /^[0-9]+[.][0-9]+$/ {
    print "invalid shard manifest row: " $0 > "/dev/stderr"
    exit 1
  }
  { print $3 }
' "$manifest" | sort >"$work/assigned"
if ! diff -u "$work/expected" "$work/assigned"; then
  printf 'Every tests/**/*.bats file must appear in exactly one shard. Update %s.\n' "$manifest" >&2
  exit 1
fi
[ "$shard" != --check ] || exit 0

selection=$(awk -v shard="$shard" '$1 == shard { print $3 }' "$manifest")
files=()
while IFS= read -r file; do
  [ -n "$file" ] && files+=("$file")
done <<<"$selection"
[ "${#files[@]}" -gt 0 ] || {
  printf 'empty shard: %s\n' "$shard" >&2
  exit 1
}
bats --timing "${files[@]}"
