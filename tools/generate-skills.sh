#!/usr/bin/env bash
# generate-skills — write each harness's copy of a skill from its one
# source, or check that the committed copies match what this would write.
#
# APM has no per-target variants: a package deploys its files verbatim.
# The Claude and Codex packages therefore each carry a full copy of every
# skill, and maintained by hand the two copies drifted in both
# directions. One source per skill, under packages/agents/source/, is
# the copy people edit; the package directories are output.
#
# A skill's source directory holds:
#
#   SKILL.<harness>.md   the manifest template for that harness. A line
#                        reading exactly `@include parts/<file>` is
#                        replaced by that file. A harness without a
#                        template gets no output from this skill.
#   shared/              copied into every harness's output
#   <harness>/           copied into that harness's output only
#   parts/               include fragments, never copied
#
# packages/agents/source/lib/lib.sh lands at scripts/lib.sh in every
# output whose scripts mention lib.sh, so each deployed skill stays
# self-contained.
#
# tokens.json is skill-tokens' output, not this script's, so writing
# leaves it in place and checking ignores it. A package skill with no
# source directory is skipped entirely: the families migrate one at a
# time.
#
# Order after any source edit or rebase: this, then `mise run
# skill-tokens`, then `apm install`. Each reads what the step before it
# wrote.
#
# Findings print one per line in the shape the vale template uses.
# Silence means a clean run.
#
# Usage: generate-skills.sh [--check]
set -euo pipefail

export LC_ALL=C
unset CDPATH GREP_OPTIONS
IFS=$' \t\n'

readonly HARNESSES="claude codex"

check=false
case ${1:-} in
"") ;;
--check) check=true ;;
*)
  printf 'usage: generate-skills.sh [--check]\n' >&2
  exit 2
  ;;
esac

root=$(git rev-parse --show-toplevel)
cd "$root"
readonly SOURCE=packages/agents/source
readonly LIB=$SOURCE/lib/lib.sh

scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT

findings=0
report() {
  printf '%s:1 [error] %s  %s\n' "$1" "$2" "$3"
  findings=$((findings + 1))
}

# render <template> <source-dir>: the template with each include line
# replaced by the fragment it names. A missing fragment fails the run
# rather than rendering a manifest with a hole in it.
render() {
  local line
  while IFS= read -r line || [ -n "$line" ]; do
    case $line in
    "@include "*)
      local part=${line#@include }
      [ -f "$2/$part" ] || {
        printf 'generate-skills: %s includes %s, which does not exist\n' "$1" "$part" >&2
        exit 1
      }
      cat "$2/$part"
      ;;
    *) printf '%s\n' "$line" ;;
    esac
  done <"$1"
}

# build <source-dir> <harness> <out>: assemble one output directory.
build() {
  local src=$1 harness=$2 out=$3 tree f rel
  mkdir -p "$out"
  render "$src/SKILL.$harness.md" "$src" >"$out/SKILL.md"
  for tree in shared "$harness"; do
    [ -d "$src/$tree" ] || continue
    while IFS= read -r f; do
      rel=${f#"$src/$tree/"}
      if [ -e "$out/$rel" ]; then
        printf 'generate-skills: %s exists in both shared/ and %s/ of %s\n' "$rel" "$harness" "$src" >&2
        exit 1
      fi
      mkdir -p "$(dirname "$out/$rel")"
      cp -p "$f" "$out/$rel"
    done < <(find "$src/$tree" -type f | sort)
  done
  if [ -d "$out/scripts" ] && grep -rqF 'lib.sh' "$out/scripts"; then
    cp -p "$LIB" "$out/scripts/lib.sh"
  fi
}

# listing <dir>: every file below it with its mode and digest, tokens.json
# aside, so two trees compare on names, bytes, and the executable bit.
listing() {
  (
    cd "$1"
    find . -type f ! -path ./tokens.json | sort | while IFS= read -r f; do
      if [ -x "$f" ]; then mode=755; else mode=644; fi
      printf '%s %s %s\n' "${f#./}" "$mode" "$(git hash-object --no-filters -- "$f")"
    done
  )
}

for src in "$SOURCE"/skills/*/; do
  src=${src%/}
  [ -d "$src" ] || continue
  name=${src##*/}
  for harness in $HARNESSES; do
    [ -f "$src/SKILL.$harness.md" ] || continue
    want="$scratch/$harness/$name"
    build "$src" "$harness" "$want"
    out="packages/agents/$harness/.apm/skills/$name"

    if [ "$check" = true ]; then
      if [ ! -d "$out" ]; then
        report "$out" not-generated "missing; run mise run generate-skills"
        continue
      fi
      while IFS= read -r line; do
        report "$out/${line%% *}" generated-drift \
          "differs from what $src generates; edit the source, then run mise run generate-skills"
      done < <(diff <(listing "$want") <(listing "$out") | sed -n 's/^[<>] //p' | cut -d' ' -f1 | sort -u)
    else
      mkdir -p "$out"
      find "$out" -mindepth 1 -maxdepth 1 ! -name tokens.json -exec rm -rf {} +
      cp -pR "$want/." "$out/"
      printf 'generated %s\n' "$out"
    fi
  done
done

if [ "$findings" -gt 0 ]; then
  printf 'TOTAL: %s finding(s)\n' "$findings"
  exit 1
fi
