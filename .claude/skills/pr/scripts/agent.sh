#!/usr/bin/env bash
# Shared unattended policy. Loaded by lib.sh, never sourced from a repository.

# Only repository behavior belongs here. Authorities still use dt_conf.
_dt_repo_conf_file() {
  local file=$1 key=$2 fallback=${3:-} value
  if [ -e "$file" ]; then
    [ -f "$file" ] && [ ! -L "$file" ] || dt_die "Repository configuration must be a regular file: $file"
    awk '
      /^[ \t]*(#|$)/ { next }
      {
        key = $0
        sub(/=.*/, "", key)
        gsub(/^[ \t]+|[ \t]+$/, "", key)
        if (!index($0, "=") || seen[key]++ ||
            key !~ /^(lint_commit_cmd|lint_task|test_task|required_hooks|hook_marker|review_max_rounds|pr_template|lint_pr_cmd|pr_log_entry|pr_related_pattern|pr_title_check_cmd|owner_paths|mail_cmd|mail_to|mail_from|lint_squash_cmd|owner_review_label|ruleset_dir|golden_paths|loosening_markers|gate_config_paths|disposable_ignored|merge_ready_cmd)$/) {
          print "Invalid or duplicate repository setting: " key > "/dev/stderr"
          bad = 1
        }
      }
      END { exit bad ? 1 : 0 }
    ' "$file" || return 1
    if value=$(_dt_conf_file "$file" "$key"); then
      printf '%s\n' "$value"
      return 0
    fi
  fi
  printf '%s\n' "$fallback"
}

dt_repo_conf() {
  local root
  root=$(dt_root) || return $?
  _dt_repo_conf_file "$root/.agents/agent-skills.conf" "$1" "${2:-}"
}

# Gate task names come from the trusted remote default. The worktree copy is
# still parsed and validated so a branch cannot publish broken configuration,
# but it cannot choose the checks that approve that same branch.
dt_gate_task() {
  local key=$1 fallback=$2 proposed ref oid blob mode file task
  proposed=$(dt_repo_conf "$key" "$fallback") || return $?
  case $proposed in
  '' | [!A-Za-z0-9]* | *[!A-Za-z0-9:._/-]*) dt_die "$key must name one nonempty mise task" ;;
  esac

  ref=$(dt_policy_ref) || return $?
  oid=$(gitr rev-parse --verify --quiet "$ref^{commit}" || printf 'none\n') || return $?
  if [ "$oid" = none ]; then
    oid=$(gitr rev-parse --verify --quiet 'HEAD^{commit}' || printf 'none\n') || return $?
  fi
  if [ "$oid" != none ] && blob=$(gitr rev-parse --verify --quiet "$oid:.agents/agent-skills.conf"); then
    mode=$(gitr ls-tree "$oid" -- .agents/agent-skills.conf) || return $?
    [ "${mode%% *}" = 100644 ] || dt_die "Trusted repository configuration must be a regular non-executable file: .agents/agent-skills.conf"
    file=$(mktemp) || return $?
    gitr cat-file blob "$blob" >"$file" || {
      rm -f "$file"
      return 1
    }
    task=$(_dt_repo_conf_file "$file" "$key" "$fallback") || {
      rm -f "$file"
      return 1
    }
    rm -f "$file"
  else
    task=$fallback
  fi
  case $task in
  '' | [!A-Za-z0-9]* | *[!A-Za-z0-9:._/-]*) dt_die "$key must name one nonempty mise task" ;;
  esac
  if [ "$proposed" != "$task" ]; then
    printf '%s is changed on this branch; using trusted task %s until the configuration lands\n' "$key" "$task" >&2
  fi
  printf '%s\n' "$task"
}

dt_agent_context() {
  local dir marker
  dir=$(dt_git_dir) || return $?
  for marker in MERGE_HEAD CHERRY_PICK_HEAD REVERT_HEAD rebase-merge rebase-apply; do
    [ ! -e "$dir/$marker" ] || dt_die "HELD: $marker is in progress; finish or abort it first."
  done
  gitr symbolic-ref --quiet HEAD >/dev/null || dt_die "HELD: HEAD is detached."
  gitr rev-parse --verify --quiet HEAD >/dev/null || dt_die "Create the initial commit through the interactive commit skill first."
}

# Persistent bounds count findings only for one review subject.
dt_agent_rounds() {
  local state subject rounds max stored_subject
  state=$(dt_state_dir) || return $?
  subject=$2
  max=$(dt_repo_conf review_max_rounds 3) || return $?
  case $max in
  [1-9]) ;;
  *) dt_die "review_max_rounds must be an integer from 1 to 9" ;;
  esac
  rounds=0
  if [ -f "$state/$1.rounds" ]; then
    stored_subject=$(sed -n 's/^subject //p' "$state/$1.rounds") || return $?
    if [ "$stored_subject" = "$subject" ]; then
      rounds=$(sed -n 's/^rounds //p' "$state/$1.rounds") || return $?
    fi
  fi
  case $rounds in
  [0-9]) ;;
  *) rounds=$max ;;
  esac
  if [ "${3:-check}" = finding ]; then
    rounds=$((rounds + 1))
    printf 'subject %s\nrounds %s\n' "$subject" "$rounds" >"$state/$1.rounds"
  fi
  if [ "$rounds" -ge "$max" ]; then
    printf 'BLOCKED: review reached the bound of %s finding rounds. Leave the draft and index in place and report the findings.\n' "$max" >&2
    return 3
  fi
}

# Use the same gate cache, with the configured command in its cache key.
dt_commit_lint() {
  local cmd
  if [ "${DT_UNATTENDED:-0}" = 1 ]; then
    cmd=$(dt_repo_conf lint_commit_cmd 'mise run lint-commit-msg') || return $?
    [ -n "$cmd" ] || dt_die "lint_commit_cmd must not be empty"
    dt_gate lint-commit-msg "$1" "$2" bash -c "$cmd"
  else
    dt_gate lint-commit-msg "$1" "$2" mise run lint-commit-msg
  fi
}

# Bind behavior to the review so changed configuration needs a fresh review.
dt_repo_digest() {
  local root
  root=$(dt_root) || return $?
  if [ -e "$root/.agents/agent-skills.conf" ]; then
    dt_digest "$root/.agents/agent-skills.conf"
  else
    printf 'none\n'
  fi
}

# Shared optional reporting transport. Delivery never retries the protected write.
dt_agent_report() {
  local kind=$1 pr=$2 text=$3 cmd to from
  cmd=$(dt_repo_conf mail_cmd) || return $?
  to=$(dt_repo_conf mail_to) || return $?
  from=$(dt_repo_conf mail_from) || return $?
  if [ -n "$cmd" ] && [ -n "$to" ]; then
    cmd=$(dt_path "$cmd") || return $?
    set -- send --to "$to" --kind "$kind" --body "$text" --pr "$pr"
    [ -z "$from" ] || set -- "$@" --from "$from"
    if ! "$cmd" "$@"; then printf 'REPORT NOT SENT: %s\n' "$text" >&2; fi
  else
    printf 'REPORT (%s): %s\n' "$kind" "$text"
  fi
}
