#!/usr/bin/env bash
set -euo pipefail

# Each record is: version <TAB> sha-or-dash <TAB> site. A version of "?"
# marks a site whose pin was found but whose version could not be read,
# which is a disagreement in its own right.
read_sites() {
  for f in mise.toml apm.yml vendir.yml .pre-commit-config.yaml \
    .github/renovate.json5 .github/workflows/*.yml .github/workflows/*.yaml; do
    [ -f "$f" ] || continue
    awk -v file="$f" '
      function emit(ver, sha) { printf "%s\t%s\t%s:%d\n", ver, sha, file, FNR }

      # APM supports the root package and the published sub-packages.
      /^[[:space:]]*-[[:space:]]*["\047]?tbhb-dev\/devtools/ && file == "apm.yml" {
        v = "?"
        if ($0 ~ /tbhb-dev\/devtools(\/packages\/agents\/(common|claude|codex|agy))?#v/) {
          v = $0; sub(/.*tbhb-dev\/devtools(\/packages\/agents\/(common|claude|codex|agy))?#/, "", v); sub(/[[:space:]"\047].*$/, "", v)
        }
        emit(v, "-")
      }

      /^"go:github.com\/tbhb-dev\/devtools\/cmd\/(guard-markdown|commit-lint|agentrepo|gh-post)"[[:space:]]*=/ {
        v = $0; sub(/^[^=]*=[[:space:]]*"/, "", v); sub(/".*$/, "", v)
        emit(v, "-")
      }

      # Reusable-workflow callers: SHA plus a trailing tag comment.
      /uses:[[:space:]]*tbhb-dev\/devtools\// {
        line = $0
        s = line; sub(/.*@/, "", s); sub(/[^0-9a-f].*$/, "", s)
        v = "?"
        if (line ~ /#[[:space:]]*v/) {
          v = line; sub(/.*#[[:space:]]*/, "", v); sub(/[[:space:]].*$/, "", v)
        }
        emit(v, s)
      }

      # The Renovate preset extends. A tag ref is the version outright; a
      # SHA ref carries its version in the comment beside it, and without
      # one the site still joins the SHA comparison.
      /github>tbhb-dev\/devtools\/\/\.github\/[^\/"]+\.json5/ {
        line = $0
        ref = line
        if (ref ~ /[^\/"]+\.json5#/) {
          sub(/.*[^\/"]+\.json5#/, "", ref); sub(/".*$/, "", ref)
        } else {
          ref = ""
        }
        if (ref == "") { emit("unpinned", "-") }
        else if (ref ~ /^v/) { emit(ref, "-") }
        else {
          v = "?"
          if (line ~ /\/\/[[:space:]]*v/) {
            v = line; sub(/.*\/\/[[:space:]]*/, "", v); sub(/[[:space:],].*$/, "", v)
          }
          emit(v, ref)
        }
      }

      # vendir.yml and .pre-commit-config.yaml name the repository one key
      # above the pin, so both are read with the block state the rewrite
      # in update-pins keeps.
      /(url|repo):[[:space:]]*https:\/\/github\.com\/tbhb-dev\/devtools([[:space:]]|$)/ { in_block = 1; next }
      in_block && /^[[:space:]]*ref:[[:space:]]*/ {
        v = $0; sub(/.*ref:[[:space:]]*/, "", v); sub(/[[:space:]].*$/, "", v)
        emit(v, "-"); in_block = 0
      }
      in_block && /^[[:space:]]*rev:[[:space:]]*/ {
        line = $0
        s = line; sub(/.*rev:[[:space:]]*/, "", s); sub(/[^0-9a-f].*$/, "", s)
        v = "?"
        if (line ~ /frozen:[[:space:]]*v/) {
          v = line; sub(/.*frozen:[[:space:]]*/, "", v); sub(/[[:space:]].*$/, "", v)
        }
        emit(v, s); in_block = 0
      }
    ' "$f"
  done
}

sites=$(read_sites)
if ! grep -Eq '^"go:github.com/tbhb-dev/devtools/cmd/agentrepo"[[:space:]]*=' mise.toml; then
  echo 'recommend declaring "go:github.com/tbhb-dev/devtools/cmd/agentrepo" in mise.toml [tools] for bounded reviews' >&2
fi

if ! grep -Eq '^"go:github.com/tbhb-dev/devtools/cmd/guard-markdown"[[:space:]]*=' mise.toml; then
  echo 'missing guard-markdown tool pin in mise.toml [tools]' >&2
  exit 1
fi
guard_version=$(sed -nE 's|^"go:github.com/tbhb-dev/devtools/cmd/guard-markdown"[[:space:]]*=[[:space:]]*"([^"]*)"|\1|p' mise.toml)
if ! grep -Eq '^"go:github.com/tbhb-dev/devtools/cmd/commit-lint"[[:space:]]*=' mise.toml; then
  printf 'missing commit-lint tool pin in mise.toml [tools]; add exactly:\n' >&2
  printf '"go:github.com/tbhb-dev/devtools/cmd/commit-lint" = "%s"\n' "$guard_version" >&2
  exit 1
fi
if ! grep -Eq '^"go:github.com/tbhb-dev/devtools/cmd/gh-post"[[:space:]]*=' mise.toml; then
  printf 'missing gh-post tool pin in mise.toml [tools]; add exactly:\n' >&2
  printf '"go:github.com/tbhb-dev/devtools/cmd/gh-post" = "%s"\n' "$guard_version" >&2
  exit 1
fi

if [ -z "$sites" ]; then
  echo "no tbhb-dev/devtools pins found -- check-pins verified nothing" >&2
  echo "         run it from a repository that vendors devtools" >&2
  exit 1
fi

printf '%s\n' "$sites" | awk -F'\t' '{ printf "  %-10s %-42s %s\n", $1, $2, $3 }'

versions=$(printf '%s\n' "$sites" | cut -f1 | sort -u)
shas=$(printf '%s\n' "$sites" | cut -f2 | grep -v '^-$' | sort -u || true)

# A disagreement reports each distinct value with the sites carrying it,
# because the site that did not move is the whole answer and reading it
# off the table above is the work this gate exists to save.
report() {
  local field=$1 label=$2 value
  echo "devtools pins disagree on the $label:" >&2
  for value in $3; do
    echo "  $value" >&2
    printf '%s\n' "$sites" | awk -F'\t' -v f="$field" -v v="$value" \
      '$f == v { print "    " $3 }' >&2
  done
}

version_count=$(printf '%s\n' "$versions" | wc -l)
sha_count=$(printf '%s\n' "$shas" | wc -l)
rc=0
if [ "$version_count" -ne 1 ] || [ "$versions" = "?" ]; then
  report 1 version "$versions"
  rc=1
fi
if [ -n "$shas" ] && [ "$sha_count" -ne 1 ]; then
  report 2 commit "$shas"
  rc=1
fi

if [ "$rc" -ne 0 ]; then
  echo "         run 'mise run devtools:update-pins <version>' to move every site together" >&2
  exit 1
fi

# Search tracked working-tree text, including adapted configuration headers.
# git grep returns 1 for no matches; other failures must still fail the gate.
mentions=$(git grep -nI -o -E 'tbhb-dev/devtools[[:space:]]+v[0-9]+\.[0-9]+\.[0-9]+' --) || {
  status=$?
  [ "$status" -eq 1 ] || exit "$status"
}
if ! printf '%s\n' "$mentions" | awk -v pin="$guard_version" '
  NF {
    mention = $0
    sub(/^.*tbhb-dev\/devtools[[:space:]]+/, "", mention)
    if (mention != pin) {
      site = $0; sub(/:tbhb-dev\/devtools[[:space:]]+v[0-9]+\.[0-9]+\.[0-9]+$/, "", site)
      printf "%s: prose mentions %s; pinned %s (edit manually)\n", site, mention, pin > "/dev/stderr"
      bad = 1
    }
  }
  END { exit bad }
'; then
  exit 1
fi

echo "every devtools pin is on $versions"
