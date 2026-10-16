#!/usr/bin/env bash
# lib — shared by the generated skills: which harness is running, the
# machine-level configuration, the script-owned independent review and
# its stamp, and the confirmation chain.
#
# Sourced, never run. Written for bash 3.2, which macOS still bundles.
# tools/generate-skills.sh copies this file into scripts/lib.sh of every
# generated skill whose scripts mention it, so each deployed skill stays
# self-contained. Edit it at packages/agents/source/lib/lib.sh; the
# copies are output.
#
# Configuration comes from the environment or the machine, never from a
# file in the repository. Every key here decides who judges the
# repository (the reviewer, the delegation checker), so a commit to the
# repository must not be able to replace the judge of the next commit.
# First hit wins:
#   1. the environment, DEVTOOLS_<KEY> in capitals
#   2. ${XDG_CONFIG_HOME:-~/.config}/devtools/<repository>.conf
#   3. ${XDG_CONFIG_HOME:-~/.config}/devtools/devtools.conf
# Each file holds `key = value` lines, with `#` comments.

# --- environment hardening -------------------------------------------
# The agent reads what the sourcing scripts print, so the operator's
# preferences must not change its shape. A CDPATH would also make the
# cd below print a path, and the directory would come back as two lines.
export LC_ALL=C
export GH_PAGER=cat
export GH_PROMPT_DISABLED=1
export PYTHONUTF8=1
unset CDPATH GH_REPO GH_HOST GREP_OPTIONS
IFS=$' \t\n'

DT_LIB_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

# Exit status for an action nobody has decided on yet: no confirmation,
# a delegation that deferred. The caller stops and asks.
# shellcheck disable=SC2034 # read by the scripts that source this file
readonly DT_HELD=75

dt_die() {
  printf '%s\n' "$*" >&2
  exit 1
}

# gitr runs git for output a script parses, with every formatting knob
# pinned. log.showSignature prepends a verification line per commit to
# stdout, so a --oneline listing silently becomes two lines per commit,
# and core.quotePath would quote a name with a byte outside ASCII, so a
# path comparison would miss it.
#
# Plain `git` stays available on purpose. Config reads, fetch, and the
# commit itself need the operator's real configuration: the sign-off
# identity may come from an includeIf work profile, and the network
# calls need credential helpers.
gitr() {
  command git --no-pager \
    -c log.showSignature=false \
    -c color.ui=false -c color.diff=false -c color.status=false \
    -c core.quotePath=false \
    -c diff.noprefix=false -c diff.mnemonicPrefix=false \
    -c diff.renames=true -c diff.context=3 \
    "$@"
}

dt_root() { gitr rev-parse --show-toplevel; }
dt_git_dir() { gitr rev-parse --absolute-git-dir; }

# The name the machine configuration knows the repository by: the
# directory of the main checkout, whichever worktree the caller is in.
dt_repo_name() {
  local common
  common=$(gitr rev-parse --path-format=absolute --git-common-dir)
  basename "$(dirname "$common")"
}

# Per worktree, so parallel agent worktrees never read each other's
# stamps. The name matches space-geese's agent-* skills, whose stamp
# format this one keeps.
dt_state_dir() {
  local dir
  dir=$(dt_git_dir)/agent-skills
  mkdir -p "$dir"
  printf '%s\n' "$dir"
}

dt_digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

# dt_head prints the commit at HEAD, or `unborn` before the first commit.
dt_head() {
  gitr rev-parse --verify --quiet HEAD || printf 'unborn\n'
}

# --- harness -----------------------------------------------------------

# dt_harness prints claude, codex, or unknown. The session variables say
# who is running the script, which is what decides the reviewer. The
# library's own path is the fallback for a run outside any session: a
# copy under .claude/skills belongs to Claude, one under .agents/skills
# to Codex.
dt_harness() {
  if [ -n "${DEVTOOLS_HARNESS:-}" ]; then
    printf '%s\n' "$DEVTOOLS_HARNESS"
  elif [ -n "${CLAUDE_CODE_SESSION_ID:-}" ] || [ -n "${CLAUDECODE:-}" ]; then
    printf 'claude\n'
  elif [ -n "${CODEX_THREAD_ID:-}" ]; then
    printf 'codex\n'
  else
    case $DT_LIB_DIR in
    */.claude/skills/* | */packages/agents/claude/*) printf 'claude\n' ;;
    */.agents/skills/* | */packages/agents/codex/*) printf 'codex\n' ;;
    *) printf 'unknown\n' ;;
    esac
  fi
}

# dt_session_id prints the id the pre-approval record is keyed on, with
# the same preference `mise run preapprove` applies when both are set.
dt_session_id() {
  printf '%s\n' "${CLAUDE_CODE_SESSION_ID:-${CODEX_THREAD_ID:-}}"
}

# --- configuration -----------------------------------------------------

# _dt_conf_file <file> <key>: print the key's value, or fail when the
# file or the key is absent.
_dt_conf_file() {
  [ -f "$1" ] || return 1
  awk -v key="$2" '
    /^[ \t]*#/ || !/=/ { next }
    {
      k = $0
      sub(/=.*/, "", k)
      gsub(/^[ \t]+|[ \t]+$/, "", k)
      if (k != key) next
      v = $0
      sub(/^[^=]*=/, "", v)
      gsub(/^[ \t]+|[ \t]+$/, "", v)
      print v
      found = 1
      exit
    }
    END { exit found ? 0 : 1 }
  ' "$1"
}

# dt_conf <key> [default]: the configured value, or the default.
dt_conf() {
  local key=$1 default=${2:-} env_name value dir
  env_name=DEVTOOLS_$(printf '%s' "$key" | tr '[:lower:]' '[:upper:]')
  if [ -n "${!env_name+set}" ]; then
    printf '%s\n' "${!env_name}"
    return 0
  fi
  dir=${XDG_CONFIG_HOME:-$HOME/.config}/devtools
  if value=$(_dt_conf_file "$dir/$(dt_repo_name).conf" "$key") ||
    value=$(_dt_conf_file "$dir/devtools.conf" "$key"); then
    printf '%s\n' "$value"
    return 0
  fi
  printf '%s\n' "$default"
}

# A configured path may start with ~/.
dt_path() {
  case $1 in
  \~/*) printf '%s\n' "$HOME/${1#??}" ;;
  *) printf '%s\n' "$1" ;;
  esac
}

# --- the independent review ----------------------------------------------
#
# A review is a fresh, read-only agent process that reads the draft cold.
# It runs the same way from an interactive session, a subagent, and a
# headless run, because it is a child process of the script and depends
# on no harness hook. The caller never handles the verdict: this code
# reads it and writes the stamp, which binds the verdict to the exact
# bytes and git state the reviewer was given.
#
# The stamp is an honesty boundary. A process running as the same user
# can write the file by hand. What it buys is that the sanctioned path
# cannot commit unreviewed bytes by accident, and that going around it
# shows in a transcript.

# dt_review_prompt <criteria-file> <arguments> <inputs-dir>: the review
# skill's own instructions without their frontmatter, with $ARGUMENTS
# filled in, followed by the gathered inputs and the reply format.
dt_review_prompt() {
  awk -v args="$2" '
    NR == 1 && /^---$/ { front = 1; next }
    front && /^---$/ { front = 0; next }
    front { next }
    {
      while ((i = index($0, "$ARGUMENTS")) > 0) {
        $0 = substr($0, 1, i - 1) args substr($0, i + 10)
      }
      print
    }
  ' "$1"
  cat <<EOF

## Inputs for this run

Arguments: $2

You run as a separate, read-only process. The output of every command
this skill lists is already in the files below, taken from one snapshot,
so read those rather than the live repository, which may have moved.
Read every one of them before you judge anything. You may also read and
search other files in the repository for context.

$(find "$3" -type f | sort)

EOF
  if [ -f "$3/operator-note.txt" ]; then
    cat <<'EOF'
## Settled by the operator

These points are decided: do not raise findings against them, but still
report anything the note does not cover, and report it if the staged
diff contradicts the note.

EOF
    cat "$3/operator-note.txt"
    printf '\n'
  fi
  cat <<'EOF'
## Reply format for this run

A script reads your reply, and no person sees it first. Reply with the
verdict block alone, as plain text: no code fence, no heading, and no
sentence before or after it. A passing reply is the single line
VERDICT: PASS and nothing else. Any other reply that isn't a findings
list under VERDICT: CHANGES REQUIRED counts as no verdict at all.
EOF
}

# dt_reviewer_label prints what reviews here, for the stamp and the
# preflight report.
dt_reviewer_label() {
  local custom
  custom=$(dt_conf review_cmd)
  if [ -n "$custom" ]; then
    printf 'review_cmd: %s\n' "$(printf '%s' "$custom" | tr '\n\r' '  ')"
  else
    printf '%s headless, model %s, effort %s\n' "$(dt_harness)" \
      "$(dt_conf review_model default)" "$(dt_conf review_effort default)"
  fi
}

# dt_run_reviewer <root> <inputs-dir>: read the prompt on standard input,
# print the reply. review_cmd replaces the default, and runs under
# bash -c in the repository root with DEVTOOLS_REVIEW_INPUTS naming the
# gathered inputs. The defaults run the harness's own CLI headless with
# nothing but read access:
#
#   claude  --tools limits the session to Read, Grep, and Glob, and
#           dontAsk refuses anything outside the allowed list instead
#           of waiting on a prompt nobody will answer. In a linked
#           worktree the inputs sit in the main checkout's git
#           directory, outside the working directory, so --add-dir
#           lets the reviewer read them.
#   codex   the read-only sandbox lets the reviewer read and search but
#           not write, and --ephemeral keeps the run out of the
#           session history.
#
# Both were probed against the installed CLIs (Claude Code 2.1.301,
# codex-cli 0.162.1): asked to create a file, neither could.
dt_run_reviewer() {
  local root=$1 inputs=$2 custom model effort
  custom=$(dt_conf review_cmd)
  model=$(dt_conf review_model)
  effort=$(dt_conf review_effort)
  if [ -n "$custom" ]; then
    (cd "$root" && DEVTOOLS_REVIEW_INPUTS=$inputs bash -c "$custom")
    return
  fi
  case $(dt_harness) in
  claude)
    set -- claude -p --no-session-persistence \
      --tools Read,Grep,Glob --allowedTools Read,Grep,Glob \
      --permission-mode dontAsk --add-dir "$inputs"
    [ -z "$model" ] || set -- "$@" --model "$model"
    [ -z "$effort" ] || set -- "$@" --effort "$effort"
    # The child is a session of its own, not this one.
    (cd "$root" && env -u CLAUDE_CODE_SESSION_ID "$@")
    ;;
  codex)
    set -- codex exec --sandbox read-only --ephemeral --color never -C "$root"
    [ -z "$model" ] || set -- "$@" -m "$model"
    [ -z "$effort" ] || set -- "$@" -c "model_reasoning_effort=\"$effort\""
    (cd "$root" && env -u CODEX_THREAD_ID "$@" - 2>/dev/null)
    ;;
  *)
    printf 'review: no harness detected, so no default reviewer applies. Set DEVTOOLS_REVIEW_CMD\nor DEVTOOLS_HARNESS (claude or codex).\n' >&2
    return 1
    ;;
  esac
}

# dt_verdict <reply-file>: print pass, changes, or unreadable.
#
# Strict, because a reply can quote the draft or the diff, and either
# can contain the words of a passing verdict. A pass is a reply whose
# only non-blank line is the verdict. Changes are a reply whose first
# non-blank line says so. Anything else is no verdict.
dt_verdict() {
  # A NUL or another control byte can end a line early for one reader
  # and not another. A reply carrying one is no verdict.
  if [ "$(tr -d '\000-\010\013\014\016-\037' <"$1" | wc -c)" -ne "$(wc -c <"$1")" ]; then
    printf 'unreadable\n'
    return 0
  fi
  awk '
    {
      sub(/\r$/, "")
      sub(/[ \t]+$/, "")
      if ($0 == "") next
      line[++n] = $0
    }
    END {
      if (n == 1 && line[1] == "VERDICT: PASS") print "pass"
      else if (n >= 1 && line[1] == "VERDICT: CHANGES REQUIRED") print "changes"
      else print "unreadable"
    }
  ' "$1"
}

# dt_new_inputs <kind>: a fresh directory for one review's gathered
# inputs, so one run never overwrites what another's reviewer is reading.
dt_new_inputs() {
  mktemp -d "$(dt_state_dir)/$1.inputs.XXXXXX"
}

# dt_review <kind> <draft> <snapshot> <criteria> <arguments> <inputs> <still-current> [name value]...
#
# Review a snapshot and stamp it on a pass. The caller gathers one
# consistent snapshot into the inputs directory first: a copy of the
# draft, and whatever else the review reads. The digest comes from that
# copy, the bytes the reviewer was given, never from the live draft.
# Each binding is a name and a value the stamp records beside the
# digest, such as the staged tree, taken when the snapshot was.
# still-current names a function of the caller's that succeeds while
# the live state still matches those bindings. Before anything is
# stamped, the live draft has to match the snapshot and that function
# has to succeed.
#   0  stamped: the review passed
#   2  changes required; revise and review again
#   4  the reviewer failed, or its reply had no verdict a script reads
#   5  the draft or what the review read changed while it ran
dt_review() {
  local kind=$1 draft=$2 snapshot=$3 criteria=$4 arguments=$5 inputs=$6 still_current=$7
  shift 7
  local state reply before verdict root
  state=$(dt_state_dir)
  reply=$state/$kind.review
  root=$(dt_root)

  [ -s "$draft" ] || dt_die "review: $draft is missing or empty"
  [ -f "$criteria" ] || dt_die "review: no review criteria at $criteria"
  [ -d "$inputs" ] || dt_die "review: no gathered inputs at $inputs"
  [ -s "$snapshot" ] || dt_die "review: no snapshot of the draft at $snapshot"

  # A review that does not pass leaves nothing behind it to commit on.
  rm -f "$state/$kind.stamp"
  before=$(dt_digest "$snapshot")

  if ! dt_review_prompt "$criteria" "$arguments" "$inputs" | dt_run_reviewer "$root" "$inputs" >"$reply"; then
    printf 'review: the reviewer failed to run; nothing stamped\n' >&2
    return 4
  fi
  if [ "$(dt_digest "$draft")" != "$before" ] || [ "$(dt_digest "$snapshot")" != "$before" ]; then
    printf 'review: %s is not the text the reviewer was given; review it again\n' "$(basename "$draft")" >&2
    return 5
  fi
  if ! "$still_current"; then
    printf 'review: what the review read changed while it ran; review it again\n' >&2
    return 5
  fi

  verdict=$(dt_verdict "$reply")
  case $verdict in
  pass)
    _dt_stamp "$state/$kind.stamp" "$before" "$@"
    printf 'VERDICT: PASS (stamped)\n'
    return 0
    ;;
  changes)
    cat "$reply"
    return 2
    ;;
  *)
    cat "$reply"
    printf 'review: no verdict a script can read; nothing stamped\n' >&2
    return 4
    ;;
  esac
}

_dt_stamp() {
  local stamp=$1 digest=$2
  shift 2
  # Bindings first, one a line. The reviewer label is free text from the
  # configuration, so it goes last, flattened onto one line, where it
  # cannot pass for a binding.
  {
    printf 'draft %s\n' "$digest"
    while [ $# -ge 2 ]; do
      printf '%s %s\n' "$1" "$(printf '%s' "$2" | tr '\n\r' '  ')"
      shift 2
    done
    printf 'at %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf 'reviewer %s\n' "$(dt_reviewer_label)"
  } >"$stamp"
}

# dt_stamp_value <kind> <name>: the value a stamp recorded, or nothing.
dt_stamp_value() {
  local stamp
  stamp=$(dt_state_dir)/$1.stamp
  [ -f "$stamp" ] || return 0
  sed -n "s/^$2 //p" "$stamp" | head -n 1
}

# dt_require_stamp <kind> <draft> [name value]...: fail unless the stamp
# covers these exact bytes and every named value still matches.
dt_require_stamp() {
  local kind=$1 draft=$2 recorded
  shift 2
  [ -f "$(dt_state_dir)/$kind.stamp" ] ||
    dt_die "No independent review has cleared $(basename "$draft"). Run the review script first."
  recorded=$(dt_stamp_value "$kind" draft)
  [ "$recorded" = "$(dt_digest "$draft")" ] ||
    dt_die "$(basename "$draft") changed after its review. Run the review script again."
  while [ $# -ge 2 ]; do
    recorded=$(dt_stamp_value "$kind" "$1")
    [ "$recorded" = "$2" ] ||
      dt_die "The $1 changed after the review (reviewed ${recorded:-nothing}, now $2). Run the review script again."
    shift 2
  done
}

# dt_clear <kind>: drop a kind's review state once its work has landed,
# along with every inputs directory its reviews left.
dt_clear() {
  local state dir
  state=$(dt_state_dir)
  rm -f "$state/$1.stamp" "$state/$1.review"
  for dir in "$state/$1".inputs.*; do
    [ -d "$dir" ] && [ ! -L "$dir" ] || continue
    rm -rf "$dir"
  done
}

# --- confirmation ----------------------------------------------------------
#
# Every write a skill makes needs someone's yes. The script resolves
# whose, in this order, so no harness hook and no skill prose has to:
#
#   1. operator     the caller passed the operator flag, which it does
#                   only after the operator answered yes this turn
#   2. preapproval  `mise run preapprove` recorded a grant of this action
#                   for this session, under <git dir>/preapprovals/<id>
#   3. delegation   the machine's delegation_check answers for the
#                   repository and the action: exit 0 grants, 1 refuses,
#                   anything else holds
#
# No source means the action is held: the caller stops and asks.

# dt_preapproval <action>: print the grant line for this session, or
# fail when there is none.
dt_preapproval() {
  local session
  session=$(dt_session_id)
  [ -n "$session" ] || return 1
  grep "^$1 " "$(dt_git_dir)/preapprovals/$session" 2>/dev/null | head -n 1 | grep .
}

# dt_delegation <action>: ask the machine's delegation checker.
#   0 granted, 1 refused, 75 deferred (no checker, or no decision)
# A broken checker never lets an action through.
dt_delegation() {
  local checker status
  checker=$(dt_path "$(dt_conf delegation_check)")
  if [ -z "$checker" ] || [ ! -x "$checker" ]; then
    return "$DT_HELD"
  fi
  status=0
  "$checker" check "$(dt_repo_name)" "$1" || status=$?
  case $status in
  0 | 1) return "$status" ;;
  *) return "$DT_HELD" ;;
  esac
}

# dt_confirm <action> <operator-flag>: resolve the confirmation for one
# action and print which source answered.
#   0 granted, 1 refused by the delegation, 75 held
dt_confirm() {
  local action=$1 operator=$2 grant status
  if [ "$operator" = true ]; then
    printf 'confirmation: operator, answered in this session\n'
    return 0
  fi
  if grant=$(dt_preapproval "$action"); then
    printf 'confirmation: pre-approval for session %s (%s)\n' "$(dt_session_id)" "${grant##* }"
    return 0
  fi
  status=0
  dt_delegation "$action" || status=$?
  case $status in
  0)
    printf 'confirmation: delegation_check granted %s in %s\n' "$action" "$(dt_repo_name)"
    return 0
    ;;
  1)
    printf 'REFUSED: the delegation check refuses %s in %s, and no operator answered.\n' "$action" "$(dt_repo_name)" >&2
    return 1
    ;;
  esac
  printf 'HELD: nobody has confirmed this %s. No operator flag, no pre-approval for this\nsession, and no delegation that grants it. Ask the operator, then run this again\nwith --confirmed.\n' "$action" >&2
  return "$DT_HELD"
}
