#!/usr/bin/env bash
# check-leaks.sh -- keep private values out of a public repository.
#
# Private literals come from exported *_PRIVATE_STRINGS variables.
# Base patterns cover private IPv4 addresses, tailnet names, password-manager
# references and private-key headers. .leak-patterns adds repository EREs;
# .leak-allow exempts matching paths from patterns, never private literals.
#
# What is scanned: file contents (binary files too, as raw bytes), file
# paths on their own, commit objects whole (message, author and committer
# lines), annotated tag objects whole, and, for a push, the ref names.
#
# A finding gives a location and the variable or rule, never the matched
# text. A location that itself matches a rule, such as a path with a
# private name in it, is replaced by an opaque identifier. Error messages
# never repeat an argument.
#
# Usage:
#   check-leaks.sh                 scan the working tree: tracked files and
#                                  untracked files git does not ignore
#   check-leaks.sh --staged        scan the index blobs of staged files
#   check-leaks.sh --range A..B    scan every object the range adds
#   check-leaks.sh --pre-push [REMOTE [URL]]
#                                  scan what a push sends. Reads the
#                                  "local-ref local-oid remote-ref
#                                  remote-oid" lines git gives a pre-push
#                                  hook on stdin and, per line, every
#                                  object reachable from the local oid
#                                  that the destination lacks. The
#                                  destination has the line's remote oid
#                                  and whatever git ls-remote reports for
#                                  URL, which is asked only when a line is
#                                  a new ref or names an oid this clone
#                                  lacks. REMOTE is ignored. With no lines
#                                  on stdin it scans everything reachable
#                                  from HEAD, so it reports old history.
#   check-leaks.sh --pre-push-hook [REMOTE [URL]]
#                                  the same for the installed hook, where
#                                  no lines means nothing to push, never
#                                  a scan of HEAD
#   check-leaks.sh FILE...         scan exactly those files
#
# Range and push scans read whole objects through git cat-file, never a
# diff, so nothing git shows differently from what it sends can hide a
# value. A missing or unreadable object, a failed git command, a shallow
# repository, or a count of objects read that differs from the count
# listed ends the scan with exit 2.
#
# Scope: this guards against accidents in a cooperative workflow. It does
# not catch a value that is URL-encoded, base64-encoded or otherwise
# transformed, a value split across lines, a literal under four characters,
# or an entry of *_PRIVATE_STRINGS that contains a comma. It does not
# stop git push --no-verify, a clone without the hook installed, or text
# typed into GitHub.
#
# Exit status: 0 clean, 1 findings, 2 misuse, or a git or read failure.
# cspell:ignore objs toread gsub nwant ngot lref loid rref roid getline tolower RSTART RLENGTH ACMRT quotepath gitlinks sprintf objecttype objectname
set -euo pipefail
export LC_ALL=C

# The exit handler reports on a saved copy of stderr: bash 3.2 can run it
# while a command's own redirection of fd 2 is still in force.
exec 3>&2

# git runs quietly: its own messages can repeat a path or a ref.
# Bash 3.2 applies errexit to `command` even when the caller tests its status.
git() { env git "$@" 2>/dev/null; }

# Repository-owned ERE allow list: patterns only, never private literals.
# Read relative to the repository root in every mode.
top=$(git rev-parse --show-toplevel || true)
pattern_exempt='^$'
if [[ -n $top && -f $top/.leak-allow ]]; then
  pattern_exempt=$(awk '!/^#/ && NF { printf "%s(%s)", sep, $0; sep="|" }' "$top/.leak-allow")
  [[ -n $pattern_exempt ]] || pattern_exempt='^$'
fi
patterns=/dev/null
if [[ -n $top && -f $top/.leak-patterns ]]; then patterns=$top/.leak-patterns; fi

finished=
die() {
  echo "check-leaks: $*" >&2
  finished=1
  exit 2
}

tmp=$(mktemp -d)
# Any other early exit is a git or read failure under set -e. Say so and
# fail, so an unreadable index or object never reads as a clean result.
# shellcheck disable=SC2329 # runs through the EXIT trap below
on_exit() {
  local rc=$?
  rm -rf "$tmp"
  if [[ -z $finished && $rc -ne 0 ]]; then
    echo "check-leaks: could not read something the scan needs; the result is incomplete" >&3
    exit 2
  fi
}
trap on_exit EXIT
needles=$tmp/needles
: >"$needles"

loaded=0

# add_needle NAME VALUE: queue one private literal. Values under four
# characters match too much to be useful, so they are skipped, loudly.
add_needle() {
  local name=$1 value=$2
  if [[ -z $value ]]; then
    return 0
  fi
  if ((${#value} < 4)); then
    echo "check-leaks: warning: $name is under 4 characters and is skipped" >&2
    return 0
  fi
  printf '%s\t%s\n' "$name" "$value" >>"$needles"
  loaded=$((loaded + 1))
}

# Every exported variable ending in _PRIVATE_STRINGS is a comma-separated
# list of private literals. mise loads these from gitignored mise.local.toml.
for name in $(compgen -e | LC_ALL=C sort); do
  [[ $name == *_PRIVATE_STRINGS ]] || continue
  IFS=',' read -r -a entries <<<"${!name}"
  index=0
  for entry in "${entries[@]+"${entries[@]}"}"; do
    index=$((index + 1))
    entry=${entry#"${entry%%[![:space:]]*}"}
    entry=${entry%"${entry##*[![:space:]]}"}
    add_needle "$name entry $index" "$entry"
  done
done

if ((loaded == 0)); then
  msg="no private values loaded (*_PRIVATE_STRINGS unset); only the publishable patterns are checked, so a host name, login or path outside those shapes passes"
  # On a machine that has the settings file, the values just didn't reach
  # this process. Say so, because that is a local setup mistake, not CI.
  if [[ -n $top && -f $top/mise.local.toml ]]; then
    msg="$msg. mise.local.toml exists here but its values are not in the environment; run the check through mise (mise run devtools:lint-leaks) so it can read them"
  fi
  echo "check-leaks: $msg" >&2
  if [[ ${GITHUB_ACTIONS-} == true ]]; then
    echo "::notice title=leak check::$msg"
  fi

fi

# Validate EREs before producers use the allow list. awk diagnostics can
# repeat a malformed expression, so replace them with a fixed error.
if ! PATTERN_EXEMPT=$pattern_exempt PATTERNS=$patterns awk '
  BEGIN {
    unused = ("" ~ ENVIRON["PATTERN_EXEMPT"])
    while ((r = (getline line < ENVIRON["PATTERNS"])) > 0) {
      if (line !~ /^#/ && line != "") unused = ("" ~ tolower(line))
    }
    exit r < 0 ? 1 : 0
  }
' >/dev/null 2>"$tmp/pattern-error"; then
  die "could not read valid pattern configuration; the result is incomplete"
fi

# scan reads the record stream the producers write and prints findings.
# A record starting with \001 opens a file (the rest is its label), one
# starting with \002 is a path to scan as text of its own, and every other
# record is "<line number or "binary"><TAB><text>".
scan() {
  PATTERN_EXEMPT=$pattern_exempt NEEDLES=$needles PATTERNS=$patterns awk '
    BEGIN {
      nf = ENVIRON["NEEDLES"]
      while ((r = (getline l < nf)) > 0) {
        t = index(l, "\t")
        nn++
        name[nn] = substr(l, 1, t - 1)
        needle[nn] = tolower(substr(l, t + 1))
      }
      # -1 is a read error, not the end of the file: without the literals the
      # scan would pass everything.
      if (r < 0) unreadable = 1
      close(nf)
      pf = ENVIRON["PATTERNS"]
      while ((r = (getline l < pf)) > 0) {
        if (l !~ /^#/ && l != "") custom[++nc] = tolower(l)
      }
      if (r < 0) unreadable = 1
      close(pf)
      exempt = "^\\.devtools/scripts/check-leaks\\.bats$|" ENVIRON["PATTERN_EXEMPT"]
      for (i = 1; i < 256; i++) ord[sprintf("%c", i)] = i
      # Split so a search for the literal finds only real uses of it.
      op_ref = "op:" "//"
      oct = "(25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])"
      dot = "\\."
      pre = "(^|[^0-9.])"
      ip10 = "10" dot oct dot oct dot oct
      ip172 = "172" dot "(1[6-9]|2[0-9]|3[01])" dot oct dot oct
      ip192 = "192" dot "168" dot oct dot oct
      ip_tailnet = "100" dot "(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])" dot oct dot oct
      re_ip = pre "(" ip10 "|" ip172 "|" ip192 ")"
      re_tailnet = pre ip_tailnet
      re_ts = "([a-z0-9-]+\\.)+ts\\.net"

    }
    # An opaque stand-in for a location that must not be printed.
    function opaque(s,    i, h) {
      h = 5381
      for (i = 1; i <= length(s); i++) h = (h * 33 + ord[substr(s, i, 1)]) % 4294967296
      return sprintf("path#%08x", h)
    }
    function report(why) {
      if (quiet) { hits++; return }
      printf "%s:%s: %s\n", label, lineno, why
      findings++
    }
    function foreign_ts(s,    m) {
      while (match(s, re_ts)) {
        m = substr(s, RSTART, RLENGTH)
        if (m != "example.ts.net" && m !~ /\.example\.ts\.net$/) return 1
        s = substr(s, RSTART + RLENGTH)
      }
      return 0
    }
    function check(low, skip_patterns,    i) {
      for (i = 1; i <= nn; i++) {
        if (index(low, needle[i])) report("private literal " name[i])
      }
      if (skip_patterns) return
      if (index(low, op_ref)) report("pattern: password-manager reference")
      if (foreign_ts(low)) report("pattern: tailnet host name")
      if (low ~ re_tailnet) report("pattern: tailnet address")
      if (low ~ re_ip) report("pattern: private IPv4 address")
      if (low ~ /-----begin ([a-z0-9]+ )*private key-----/) report("pattern: private-key header")
      for (i = 1; i <= nc; i++) {
        if (low ~ custom[i]) report("pattern: repository rule " i)
      }
    }
    # Name a location: as given, unless the name itself matches a rule.
    function locate(full,    n) {
      quiet = 1; hits = 0
      # Exemptions silence findings, never the sanitizing of a location.
      check(tolower("/" full), 0)
      quiet = 0
      return hits ? opaque(full) : full
    }
    # With the private values unread, say nothing: any finding or location
    # would come from an incomplete set, and the END rule reports the failure.
    unreadable { next }
    /^\001/ { full = substr($0, 2); label = locate(full); skip = (full ~ exempt); next }
    /^\002/ {
      full = substr($0, 2)
      label = locate(full)
      lineno = "path"
      # The leading slash lets the path patterns see a top-level directory.
      check(tolower("/" full), (full ~ exempt))
      next
    }
    {
      t = index($0, "\t")
      lineno = substr($0, 1, t - 1)
      check(tolower(substr($0, t + 1)), skip)
    }
    END {
      if (unreadable) { print "check-leaks: could not read the private values; the result is incomplete" > "/dev/stderr"; exit 2 }
      if (findings) printf "check-leaks: %d finding(s); values are not printed\n", findings > "/dev/stderr"
      exit findings ? 1 : 0
    }
  ' 2>"$tmp/scan-error"
}

# esc makes a path safe for the line-based record stream: git hands paths
# over NUL-delimited, and a newline or a backslash in one is escaped here.
esc() {
  local s=$1
  s=${s//\\/\\\\}
  s=${s//$'\n'/\\n}
  s=${s//$'\r'/\\r}
  printf '%s' "$s"
}

# has_nul FILE: true when the file holds a NUL byte.
has_nul() {
  # fd 2 goes first so a failed open of the file can't print its name.
  # shellcheck disable=SC2312 # emit_body reads the file next, and fails there on one that can't be read
  (($(tr -d '\0' 2>/dev/null <"$1" | wc -c) != $(wc -c 2>/dev/null <"$1")))
}

# emit_body FILE: the file's content as numbered records. NULs become
# line breaks so a binary blob is searched as raw bytes, and its lines are
# marked "binary" because their numbers mean nothing.
emit_body() {
  local mark=
  if has_nul "$1"; then mark=binary; fi
  tr '\0' '\n' 2>/dev/null <"$1" | awk -v mark="$mark" '{ print (mark == "" ? NR : mark) "\t" $0 }'
}

# emit_path PATH: a path as text of its own.
emit_path() {
  # shellcheck disable=SC2312 # esc only rewrites the text
  printf '\002%s\n' "$(esc "$1")"
}

# emit_file LABEL FILE: a path and the file's content.
emit_file() {
  emit_path "$1"
  # shellcheck disable=SC2312 # esc only rewrites the text
  printf '\001%s\n' "$(esc "$1")"
  emit_body "$2"
}

emit_tree() {
  local f tree_skip
  load_leak_skip
  git ls-files -z --cached --others --exclude-standard >"$tmp/list"
  while IFS= read -r -d '' f; do
    [[ $f =~ $tree_skip ]] && continue
    if [[ -f $f && ! -L $f ]]; then
      emit_file "$f" "$f"
    elif [[ -L $f ]]; then
      readlink "$f" >"$tmp/link"
      emit_file "$f" "$tmp/link"
    elif [[ -e $f ]]; then
      emit_path "$f"
    fi
  done <"$tmp/list"
}

# emit_raw FILE MODE: the paths of a `diff --raw -z -M` listing, with both
# sides of a rename or copy. With MODE content it also emits each new blob
# whole; with any other MODE, only the paths.
emit_raw() {
  local meta st p1 p2 p
  local -a m
  while IFS= read -r -d '' meta; do
    read -r -a m <<<"${meta#:}"
    st=${m[4]}
    IFS= read -r -d '' p1
    p=$p1
    if [[ $st == R* || $st == C* ]]; then
      emit_path "$p1"
      IFS= read -r -d '' p2
      p=$p2
    fi
    if [[ ${m[1]} == 160000 ]]; then
      emit_path "$p"
      continue
    fi
    if [[ $2 == content ]]; then
      git cat-file blob "${m[3]}" >"$tmp/blob"
      emit_file "$p" "$tmp/blob"
    else
      emit_path "$p"
    fi
  done <"$1"
}

emit_staged() {
  git diff --cached --raw -z -M --diff-filter=ACMRT >"$tmp/raw"
  emit_raw "$tmp/raw" content
}

# emit_objects: the object ids in $tmp/objs, one "oid[ path]" per line as
# rev-list --objects prints them, scanned whole.
#
# rev-list gives an object only the first path it meets, so the paths come
# from the trees instead: every entry of every listed commit's tree, with
# gitlinks listed but not entered. Each path is scanned as text of its own,
# and an object in the scanned set is pattern-scanned unless every path
# that uses it is exempt. An object the destination already has is out of
# scope, by design. Every object is then read through one cat-file --batch and
# scanned in full; a tree is read too, because the names inside it are
# what shows a file renamed without any change to its content.
emit_objects() {
  local c
  awk '!seen[$1]++' "$tmp/objs" >"$tmp/uniq"
  awk '{ print $1 }' "$tmp/uniq" | git cat-file --batch-check='%(objectname) %(objecttype)' >"$tmp/types"
  # Anything but a tree, blob, commit or tag (a "missing" answer included)
  # is an object the scan could not account for.
  if awk '$2 != "blob" && $2 != "tree" && $2 != "commit" && $2 != "tag" { bad = 1 } END { exit !bad }' "$tmp/types"; then
    return 1
  fi
  awk '$2 == "commit" { print $1 }' "$tmp/types" >"$tmp/commits"
  while read -r c; do
    git -c core.quotepath=off ls-tree -r -t --full-tree "$c"
  done <"$tmp/commits" |
    awk '{ t = index($0, "\t"); split(substr($0, 1, t - 1), f, " "); print f[3] " " substr($0, t + 1) }' |
    awk '!seen[$0]++' >"$tmp/pairs"
  # Paths, escaped for the record stream: those from the trees, then any
  # rev-list found for an object no commit's tree holds (a tagged blob).
  cat "$tmp/pairs" "$tmp/uniq" |
    awk 'index($0, " ") { p = substr($0, index($0, " ") + 1); gsub(/\\/, "\\\\", p); if (!(p in seen)) { seen[p] = 1; print "\002" p } }'
  awk '{ print $1 }' "$tmp/types" >"$tmp/toread"
  git cat-file --batch <"$tmp/toread" >"$tmp/batch"
  # A header line is "<oid> <type> <size>". Object bodies are scanned as
  # text and never interpreted as protocol: a missing object was already
  # refused by the type check above, and one that vanishes since leaves a
  # header unseen, which the count catches. A body can't forge a pass,
  # since every line is scanned whichever object it is credited to.
  tr '\0' '\n' <"$tmp/batch" | PAIRS=$tmp/pairs NAMES=$tmp/uniq TYPES=$tmp/types PATTERN_EXEMPT=$pattern_exempt awk '
    function load(file, kind,    r, l, sp, p, o) {
      while ((r = (getline l < file)) > 0) {
        if (kind == "types") { split(l, f, " "); want[f[1]] = f[2]; nwant++; continue }
        sp = index(l, " ")
        if (!sp) continue
        o = substr(l, 1, sp - 1)
        p = substr(l, sp + 1)
        # Prefer a path that is not exempt, so patterns run if any use is.
        if (!(o in chosen) || (chosen[o] ~ exempt && p !~ exempt)) chosen[o] = p
      }
      if (r < 0) bad = 1
      close(file)
    }
    BEGIN {
      exempt = "^\\.devtools/scripts/check-leaks\\.bats$|" ENVIRON["PATTERN_EXEMPT"]
      load(ENVIRON["TYPES"], "types")
      load(ENVIRON["PAIRS"], "paths")
      load(ENVIRON["NAMES"], "paths")
    }
    (NF == 3 && $1 in want && !($1 in got) && $2 == want[$1] && $3 ~ /^[0-9]+$/) {
      oid = $1
      got[oid] = 1
      ngot++
      n = 0
      if (($2 == "blob" || $2 == "tree") && chosen[oid] != "") { label = chosen[oid]; gsub(/\\/, "\\\\", label) }
      else label = $2 " " substr(oid, 1, 7)
      printf "\001%s\n", label
      next
    }
    { n++; print n "\t" $0 }
    END { if (bad || ngot != nwant) exit 3 }
  '
}

# add_objects REV-LIST-ARGS...: list the objects git would reach.
add_objects() {
  git rev-list --objects "$@" >>"$tmp/objs"
}

# Refuse a shallow repository: its boundary commits hide history.
require_complete() {
  local shallow
  shallow=$(git rev-parse --is-shallow-repository)
  if [[ $shallow != false ]]; then
    die "the repository is shallow, so the scan can't be complete"
  fi
}

emit_range() {
  require_complete
  : >"$tmp/objs"
  add_objects "$1"
  emit_objects
}

# emit_pre_push URL: the lines git gives a pre-push hook are
# "local-ref local-oid remote-ref remote-oid". For each, scan the objects
# reachable from the local oid minus those the destination already has.
# The destination has the line's own remote oid, when that object exists
# here. When any line is a new ref, or names a remote oid this clone lacks,
# the destination is asked what it has: git ls-remote against the URL git
# passed the hook, which is the exact place the push goes. Remote-tracking
# refs are a cache of some other moment and play no part. If the question
# fails the scan fails, since the push would too. The URL is never printed.
emit_pre_push() {
  local url=${1-} hook=${2-} lref loid rref roid extra count=0 ask='' oid
  local re='^([0-9a-f]{40}|[0-9a-f]{64})$'
  require_complete
  : >"$tmp/objs"
  : >"$tmp/lines"
  : >"$tmp/known"
  if [[ ! -t 0 ]]; then
    while read -r lref loid rref roid extra || [[ -n ${lref-} ]]; do
      if [[ -z ${lref-} && -z ${loid-} ]]; then continue; fi
      if [[ -n ${extra-} || ! $loid =~ $re || ! $roid =~ $re ]]; then
        die "a ref line from git is malformed"
      fi
      count=$((count + 1))
      # Ref names are text that gets published.
      # shellcheck disable=SC2312 # esc only rewrites the text
      printf '\002%s\n' "$(esc "$lref")" "$(esc "$rref")"
      # A deletion sends no objects.
      if [[ ! $loid =~ ^0+$ ]]; then
        printf '%s %s\n' "$loid" "$roid" >>"$tmp/lines"
        if [[ $roid =~ ^0+$ ]] || ! git cat-file -e "$roid"; then ask=1; fi
      fi
      lref=
    done
  fi
  if ((count == 0)); then
    # A hook run with no ref lines has nothing to push. A manual run has no
    # lines either, and scans everything reachable from HEAD.
    if [[ -n $hook ]]; then return 0; fi
    add_objects HEAD
  else
    if [[ -n $ask && -n $url ]]; then
      GIT_TERMINAL_PROMPT=0 git ls-remote "$url" >"$tmp/ls"
      while read -r oid _; do
        if [[ $oid =~ $re ]] && git cat-file -e "$oid"; then
          printf '^%s\n' "$oid" >>"$tmp/known"
        fi
      done <"$tmp/ls"
    fi
    while read -r loid roid; do
      {
        printf '%s\n' "$loid"
        cat "$tmp/known"
        if [[ ! $roid =~ ^0+$ ]] && git cat-file -e "$roid"; then printf '^%s\n' "$roid"; fi
      } >"$tmp/revs"
      git rev-list --objects --stdin <"$tmp/revs" >>"$tmp/objs"
    done <"$tmp/lines"
  fi
  emit_objects
}

mode=tree
range=
push_url=
hook=
files=()
while (($#)); do
  case $1 in
  --staged) mode=staged ;;
  --range)
    shift
    (($#)) || die "--range needs A..B"
    mode=range
    range=$1
    ;;
  --pre-push | --pre-push-hook)
    mode=pre-push
    if [[ $1 == --pre-push-hook ]]; then hook=1; fi
    # git gives the hook the remote's name, then the URL it pushes to.
    if (($# > 1)) && [[ $2 != -* ]]; then shift; fi
    if (($# > 1)) && [[ $2 != -* ]]; then
      push_url=$2
      shift
    fi
    ;;
  -*) die "unknown option" ;;
  *)
    mode=files
    files+=("$1")
    ;;
  esac
  shift
done

script_dir=$(dirname "${BASH_SOURCE[0]}")
# shellcheck source=.devtools/scripts/leak-skip.sh
if [[ $mode == tree ]]; then source "$script_dir/leak-skip.sh"; fi
if [[ $mode != files ]]; then
  top=$(git rev-parse --show-toplevel)
  cd "$top"
fi

case $mode in
tree) emit_tree >"$tmp/stream" ;;
staged) emit_staged >"$tmp/stream" ;;
range) emit_range "$range" >"$tmp/stream" ;;
pre-push) emit_pre_push "$push_url" "$hook" >"$tmp/stream" ;;
files)
  for f in "${files[@]}"; do
    [[ -f $f ]] || die "an argument is not a readable file"
  done
  for f in "${files[@]}"; do emit_file "$f" "$f"; done >"$tmp/stream"
  ;;
esac

rc=0
scan <"$tmp/stream" || rc=$?
if ((rc > 1)); then
  echo "check-leaks: could not complete the scan; the result is incomplete" >&2
  rc=2
fi
finished=1
exit "$rc"
