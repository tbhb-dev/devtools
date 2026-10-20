#!/usr/bin/env bash
# squash-message — brief whoever writes the squash commit message, then
# leave them a skeleton to write into.
#
# A squash message is not a reformatted pull request description. It is
# the one commit a whole branch leaves behind, and it answers to someone
# running git log years from now who has neither the description nor the
# review thread beside them. Writing it means reading everything that
# landed and deciding what still matters. That is a writing exercise,
# not a transformation, and no script performs it. This one does not
# try.
#
# What a script can do is put every input in front of the writer at
# once: the description as published, every commit message the squash
# collapses, the diffstat, and the trailers those commits carry. Then it
# writes the parts that are mechanical rather than editorial — the
# subject, whose reference has to name this pull request, and the
# footer, whose closing keywords and trailers come from elsewhere — and
# leaves the body empty for the writer.
#
# Nothing caps the commit listing. A cap is what turns a briefing into a
# misleading sample: the reader cannot tell a branch of eight commits
# from the first eight of forty, and the whole question here is what one
# message owes to everything that actually landed.
#
# Every fact comes from gh rather than from the local repository. This
# skill also merges pull requests nobody here authored, a dependency
# bump being the usual case, and those branches are often not checked
# out at all.
#
# Usage: squash-message.sh [pull-request-number]
# Written to bash 3.2.
set -euo pipefail

# --- environment hardening -------------------------------------------
# The agent reads this output, so the operator's preferences must not
# change its shape. LC_ALL pins collation, because sort and the [a-z]
# ranges below mean different things under a UTF-8 locale. The unsets
# cover variables that silently retarget a command: GH_REPO sends gh at
# another repository, CDPATH makes a relative cd print somewhere else.
export LC_ALL=C
export GH_PAGER=cat
export GH_PROMPT_DISABLED=1
export PYTHONUTF8=1
unset CDPATH GH_REPO GH_HOST GREP_OPTIONS
IFS=$' \t\n'

# shellcheck disable=SC1091 # generated beside this script
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

readonly DRAFT=SQUASH_AGENTMSG

root=$(git rev-parse --show-toplevel)
cd "$root"

die() {
  printf 'squash-message: %s\n' "$1" >&2
  exit 1
}

section() { printf '\n== %s ==\n' "$1"; }
none() { printf '(none)\n'; }

command -v gh >/dev/null 2>&1 || die "gh is not installed"
command -v commit-lint >/dev/null 2>&1 ||
  die 'commit-lint is not installed; add "go:github.com/tbhb-dev/devtools/cmd/commit-lint" = "<devtools-version>" to mise.toml [tools]'
signoff_identity=$(dt_signoff_identity) || exit $?
dt_gh_login >/dev/null || exit $?

number=${1:-}
if [ -z "$number" ]; then
  branch=$(git rev-parse --abbrev-ref HEAD)
  number=$(dt_gh pr list --head "$branch" --state open --json number --jq '.[0].number') || exit $?
  [ -n "$number" ] || die "no open pull request for ${branch}. Pass a number explicitly."
fi

# Read the briefing from one response so its fields describe one snapshot.
briefing=$(dt_gh pr view "$number" --json title,baseRefName,headRefName,changedFiles,additions,deletions,body,commits,files) || exit $?
meta=$(printf '%s\n' "$briefing" | jq -r '[.title, .baseRefName, .headRefName, .changedFiles, .additions, .deletions] | @tsv')
IFS=$(printf '\t') read -r title base head_ref changed added removed <<EOF
$meta
EOF
[ -n "$title" ] || die "pull request #${number} has no title"

body=$(printf '%s\n' "$briefing" | jq -r '.body')

# Each record opens with a --- line, so the trailer pass below can tell
# one commit's message from the next.
commits=$(printf '%s\n' "$briefing" | jq -r '.commits[] | "--- " + .oid[0:7] + "  " + (.authors[0].name // "unknown")
        + "\n" + .messageHeadline
        + (if (.messageBody // "") == "" then "" else "\n\n" + .messageBody end)
        + "\n"') || exit $?
count=$(printf '%s\n' "$commits" | grep -c '^--- ' || true)

files=$(printf '%s\n' "$briefing" | jq -r '.files[]? | [.additions, .deletions, .path] | @tsv') || exit $?

# --- trailers --------------------------------------------------------
#
# Taken from the commits rather than the description, because that is
# where the sign-off actually happened. This walks each record backwards
# from its final non-blank line, collecting trailer-shaped lines until
# it reaches one that is not.
#
# The walk crosses blank lines on purpose, which git's own trailer
# parser does not. Commits in these repositories routinely separate
# Assisted-by from Signed-off-by with one, and a walk that stopped there
# would drop the attribution on every squash. The cost is that a body
# whose last line happens to read `Word: something` can be swept in, and
# that lands visibly in the footer where the writer sees it.
#
# Stopping short of line 1 covers a commit with no body: its subject
# reads as `type: description`, which is trailer-shaped, and every
# one-line commit would otherwise contribute its own subject.
trailers=$(printf '%s\n' "$commits" | awk '
  function flush(   i, p, hits) {
    p = n
    hits = 0
    while (p > 1) {
      if (buf[p] ~ /^[ \t]*$/) { p--; continue }
      if (buf[p] !~ /^[A-Za-z][A-Za-z0-9-]*:[ \t]+[^ \t]/) break
      out[++hits] = buf[p]
      p--
    }
    for (i = hits; i >= 1; i--) print out[i]
    n = 0
  }
  /^--- / { flush(); next }
  { buf[++n] = $0 }
  END { flush() }
' | awk '!seen[$0]++')

# Validate carried sign-offs through the same repository identity rules.
filtered=""
while IFS= read -r trailer; do
  case $trailer in
  [Ss][Ii][Gg][Nn][Ee][Dd]-[Oo][Ff][Ff]-[Bb][Yy]:*)
    identity=${trailer#*:}
    identity=${identity# }
    if commit-lint --check-signoff "$identity"; then
      :
    else
      status=$?
      [ "$status" -eq 1 ] || exit "$status"
      printf 'Dropped sign-off: %s\n' "$trailer" >&2
      continue
    fi
    ;;
  esac
  filtered="${filtered}${trailer}"$'\n'
done <<<"$trailers"
trailers=$filtered
if ! grep -Fqx -- "Signed-off-by: $signoff_identity" <<<"$trailers"; then
  trailers="${trailers}Signed-off-by: ${signoff_identity}"$'\n'
fi

# Order follows the shared trailer rule: attribution first, sign-off
# last. Anything else this repository's rule permits sits between them,
# where neither constraint reaches it.
ordered=$(printf '%s\n' "$trailers" | awk '
  NF == 0 { next }
  /^Assisted-by:/ { a[++na] = $0; next }
  /^Signed-off-by:/ { s[++ns] = $0; next }
  { o[++no] = $0 }
  END {
    for (i = 1; i <= na; i++) print a[i]
    for (i = 1; i <= no; i++) print o[i]
    for (i = 1; i <= ns; i++) print s[i]
  }
')

# --- closing references ----------------------------------------------
#
# Taken by section name rather than by position, so a description that
# grew an extra heading still yields its Related block.
section_of() {
  printf '%s\n' "$body" | awk -v want="$1" '
    /^##[ \t]/ { inside = (tolower($0) ~ tolower("^## " want "[ \t]*$")); next }
    inside { print }
  '
}

# The leading-character class keeps a cross-repository reference out of
# the footer. `tbhb/other#99` reduced to `#99` would close whichever
# unrelated issue holds that number here, and a closing keyword acts on
# merge without asking.
related=$(section_of related |
  grep -oE '(^|[^[:alnum:]_/])#[0-9]+' | grep -oE '[0-9]+' |
  sort -u -n | awk -v self="$number" '$0 != self' || true)

# Related lists whatever the author put there: the base of a stack, a
# companion pull request, an issue that closed long ago. A closing
# keyword on any of those is wrong, so GitHub is asked what each number
# is, and only an open issue gets one. The issues endpoint answers for
# pull requests too, and marks them with a pull_request key. Every
# other number, and one GitHub would not describe, goes in a note under
# the skeleton for the writer, outside the message.
closes=""
held=""
for ref in $related; do
  kind=$(dt_gh_repo "query(\$owner:String!,\$name:String!,\$number:Int!){repository(owner:\$owner,name:\$name){issueOrPullRequest(number:\$number){__typename ... on Issue{state}}}}" -F "number=$ref" \
    --jq '.data.repository.issueOrPullRequest | if .__typename == "PullRequest" then "pull request" elif .__typename == "Issue" then (.state | ascii_downcase) + " issue" else "" end') || exit $?
  case $kind in
  "open issue") closes="${closes}Closes #${ref}"$'\n' ;;
  "") held="${held}# Not closed: #${ref} could not be read from GitHub"$'\n' ;;
  *) held="${held}# Not closed: #${ref} is a ${kind}"$'\n' ;;
  esac
done
closes=${closes%$'\n'}
held=${held%$'\n'}

# --- the skeleton ----------------------------------------------------
#
# GitHub appends the number to the subject on a squash merge only when
# it writes the message itself. This workflow supplies the message, so
# the reference is ours to add, and squash-merge.sh reads it back to
# confirm the draft belongs to the pull request being merged.
subject="${title} (#${number})"

{
  printf '%s\n' "$subject"
  if [ -n "$closes" ]; then printf '\n%s\n' "$closes"; fi
  if [ -n "$ordered" ]; then printf '\n%s\n' "$ordered"; fi
} >"$DRAFT"

# --- the briefing ----------------------------------------------------

section "the pull request"
printf '#%s %s\n' "$number" "$title"
printf '%s into %s\n' "$head_ref" "$base"
printf '%s commits, %s files changed, %s insertions, %s deletions\n' \
  "$count" "$changed" "$added" "$removed"

section "description as published"
if [ -n "$body" ]; then printf '%s\n' "$body"; else none; fi

section "commits being collapsed, in full"
if [ -n "$commits" ]; then printf '%s\n' "$commits"; else none; fi

section "diffstat"
if [ -n "$files" ]; then
  printf '%s\n' "$files" | awk -F'\t' 'NF { printf "%7s+ %7s-  %s\n", $1, $2, $3 }'
  # The files list arrives paginated, so a wide branch can report fewer
  # rows than it changed. Say so rather than letting the listing read as
  # the whole diff.
  listed=$(printf '%s\n' "$files" | grep -c . || true)
  if [ "$listed" != "$changed" ]; then
    printf '\n%s of %s files listed; gh truncated the rest.\n' "$listed" "$changed"
  fi
else
  none
fi

section "trailers across the commits"
if [ -n "$ordered" ]; then printf '%s\n' "$ordered"; else none; fi

section "$DRAFT as written"
cat "$DRAFT"
# Printed here, never written into the draft: GitHub keeps every line of
# the message it receives, comment lines included.
if [ -n "$held" ]; then printf '\n%s\n' "$held"; fi

cat <<'NOTE'

--- what is missing

The body. Everything above it is mechanical and everything in it is
not, so the skeleton stops where the writing starts.

Write the body between the subject and the footer. It accounts for the
whole stack of commits above in plain prose: the problem the branch
solves, the constraint that shaped the approach, what breaks without
it. Summary and Why from the description are the raw material. Discard
Verification, which reported what a reviewer needed at the time. Keep
Risk only where it names a rollback the reader would not guess.

Nothing merges until review-squash-message clears the result, and an
empty body fails that review by definition.
NOTE
