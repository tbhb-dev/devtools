#!/usr/bin/env bash
# Frozen configuration oracle from 1179d47 for old/new parity only.
# Keep this independent of the production wrappers.
dt_die() {
  printf '%s\n' "$*" >&2
  exit 1
}

gitr() {
  env git --no-pager \
    -c log.showSignature=false \
    -c color.ui=false -c color.diff=false -c color.status=false \
    -c core.quotePath=false \
    -c diff.noprefix=false -c diff.mnemonicPrefix=false \
    -c diff.renames=true -c diff.context=3 \
    "$@"
}

dt_repo_name() {
  local common parent
  common=$(gitr rev-parse --path-format=absolute --git-common-dir) || return $?
  parent=$(dirname "$common") || return $?
  basename "$parent"
}

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

dt_conf() {
  local key=$1 default=${2:-} env_name value dir repo
  env_name=DEVTOOLS_$(printf '%s' "$key" | tr '[:lower:]' '[:upper:]')
  if [ -n "${!env_name+set}" ]; then
    printf '%s\n' "${!env_name}"
    return 0
  fi
  dir=${XDG_CONFIG_HOME:-$HOME/.config}/devtools
  repo=$(dt_repo_name) || return $?
  if value=$(_dt_conf_file "$dir/$repo.conf" "$key") ||
    value=$(_dt_conf_file "$dir/devtools.conf" "$key"); then
    printf '%s\n' "$value"
    return 0
  fi
  printf '%s\n' "$default"
}

dt_root() { gitr rev-parse --show-toplevel; }

dt_repo_conf() {
  local root file value
  root=$(dt_root) || return $?
  file=$root/.agents/agent-skills.conf
  if [ -e "$file" ]; then
    [ -f "$file" ] && [ ! -L "$file" ] || dt_die "Repository configuration must be a regular file: $file"
    awk '
      /^[ \t]*(#|$)/ { next }
      {
        key = $0
        sub(/=.*/, "", key)
        gsub(/^[ \t]+|[ \t]+$/, "", key)
        if (!index($0, "=") || seen[key]++ ||
            key !~ /^(lint_commit_cmd|required_hooks|hook_marker|review_max_rounds|pr_template|lint_pr_cmd|pr_log_entry|pr_related_pattern|pr_title_check_cmd|owner_paths|mail_cmd|mail_to|mail_from|lint_squash_cmd|owner_review_label|ruleset_dir|golden_paths|loosening_markers|gate_config_paths|disposable_ignored|merge_ready_cmd)$/) {
          print "Invalid or duplicate repository setting: " key > "/dev/stderr"
          bad = 1
        }
      }
      END { exit bad ? 1 : 0 }
    ' "$file" || return 1
    if value=$(_dt_conf_file "$file" "$1"); then
      printf '%s\n' "$value"
      return 0
    fi
  fi
  printf '%s\n' "${2:-}"
}

"$@"
