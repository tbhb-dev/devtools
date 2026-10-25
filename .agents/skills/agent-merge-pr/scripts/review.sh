#!/usr/bin/env bash
set -euo pipefail
export DEVTOOLS_UNATTENDED=1
#!/usr/bin/env bash
# Inlined into merge entry scripts by generate-skills; never sourced from the checkout.
export LC_ALL=C GH_PAGER=cat GH_PROMPT_DISABLED=1 PYTHONUTF8=1
unset CDPATH GH_REPO GH_HOST GREP_OPTIONS
IFS=$' \t\n'

# Threat model: the invoked entry script, Git/GitHub, tools, machine files and
# pinned base are trusted. A divergent checkout may change or add any helper;
# it cannot choose the verifier or dispatcher, or supply code to the snapshot.
# Both skill directories must match the base before helpers load and at the
# final gate. Helpers run from a private base snapshot throughout the process.
# Editing this entry script, replacing trusted tools/objects, and concurrent
# filesystem attacks by the same OS user remain outside this boundary.
merge_bootstrap() {
  local enabled=${DEVTOOLS_UNATTENDED-${DT_UNATTENDED:-0}} arg number='' branch base repository relative parent
  local entry_dir entry_name mode path
  entry_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd) || return $?
  entry_name=${BASH_SOURCE[0]##*/}
  MERGE_LIB=$entry_dir/lib.sh
  MERGE_WRAPPER=false
  case $entry_dir in
  */agent-merge-pr/scripts)
    MERGE_WRAPPER=true
    enabled=1
    entry_dir=$(cd "$entry_dir/../../merge-pr/scripts" && pwd) || return $?
    ;;
  esac
  if [ "$entry_name" = squash-merge.sh ]; then
    enabled=1
    for arg in "$@"; do
      if [ "$arg" = --confirmed ] && [ "${DEVTOOLS_UNATTENDED-${DT_UNATTENDED:-0}}" != 1 ]; then enabled=0; fi
    done
  fi
  [ "$enabled" = 1 ] || return 0
  while [ "$#" -gt 0 ]; do
    case $1 in
    --note) shift ;;
    '' | *[!0-9]*) ;;
    *) number=$1 ;;
    esac
    shift
  done
  if [ -z "$number" ] && [ -s SQUASH_AGENTMSG ]; then
    number=$(sed -n '1s/.*(#\([0-9][0-9]*\))[[:space:]]*$/\1/p' SQUASH_AGENTMSG) || return $?
  fi
  if [ -z "$number" ]; then
    branch=$(git rev-parse --abbrev-ref HEAD) || return $?
    number=$(gh pr list --head "$branch" --state open --json number --jq '.[0].number // empty') || return $?
  fi
  [ -n "$number" ] || {
    printf 'merge integrity: no pull request found\n' >&2
    return 1
  }
  base=$(gh pr view "$number" --json baseRefName --jq .baseRefName) || return $?
  base=$(gh api graphql -F owner='{owner}' -F name='{repo}' -f "query=query(\$owner:String!,\$name:String!,\$ref:String!){repository(owner:\$owner,name:\$name){ref(qualifiedName:\$ref){target{oid}}}}" -f "ref=refs/heads/$base" --jq '.data.repository.ref.target.oid // error("base ref not found")') || return $?
  if ! git cat-file -e "$base^{commit}" 2>/dev/null; then
    repository=$(gh repo view --json url --jq .url) || return $?
    git -c credential.helper= -c 'credential.helper=!gh auth git-credential' fetch --no-tags --no-write-fetch-head "$repository" "$base" || return $?
  fi
  relative=$(git -C "$entry_dir" rev-parse --show-prefix) || return $?
  relative=${relative%/scripts/}
  parent=${relative%/merge-pr}
  MERGE_SNAPSHOT=$(mktemp -d "${TMPDIR:-/tmp}/devtools-merge.XXXXXX") || return $?
  MERGE_STATUS=0
  trap 'MERGE_STATUS=$?; rm -rf -- "$MERGE_SNAPSHOT"; exit "$MERGE_STATUS"' EXIT
  git ls-tree -rz "$base" -- "$relative" "$parent/agent-merge-pr" >"$MERGE_SNAPSHOT/files" || return $?
  while IFS=$' \t' read -r -d '' mode _ _ path; do
    mkdir -p "$MERGE_SNAPSHOT/${path%/*}" || return $?
    git cat-file blob "$base:$path" >"$MERGE_SNAPSHOT/$path" || return $?
    if [ "$mode" = 100755 ]; then chmod +x "$MERGE_SNAPSHOT/$path" || return $?; fi
  done <"$MERGE_SNAPSHOT/files"
  MERGE_LIB=$MERGE_SNAPSHOT/$relative/scripts/lib.sh
  uv run --quiet --script "$MERGE_SNAPSHOT/$relative/scripts/merge-policy.py" integrity "$base" "$entry_dir" || return $?
  # shellcheck disable=SC2034 # the base library uses this path at each integrity gate
  readonly DT_MERGE_CHECKOUT=$entry_dir
}
merge_bootstrap "$@" || exit $?
if [ "$MERGE_WRAPPER" = false ]; then
  # shellcheck disable=SC1090 # only the verified base snapshot supplies unattended helpers
  . "$MERGE_LIB"
fi
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
bash "$here/../../merge-pr/scripts/review.sh" "$@"
