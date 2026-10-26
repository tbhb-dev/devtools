#!/usr/bin/env bash
# Synthetic repository-configuration reader for the consumer contract tests.
# It accepts only the settings listed in _allowed and, when _required is
# set, rejects a conf that lacks any of them, which is how a release makes a
# key required. It also accepts the mode setting at one exact value. The
# real readers are measured by the baseline test instead.

_allowed=" alpha beta mode "
_required=""
_pinned_mode="old"

dt_root() { git rev-parse --show-toplevel; }

_trim() {
  local text=$1
  text=${text#"${text%%[![:space:]]*}"}
  text=${text%"${text##*[![:space:]]}"}
  printf '%s' "$text"
}

dt_repo_conf() {
  local file line key value found="" wanted mode_value
  file=$(dt_root)/.agents/agent-skills.conf
  value=${2:-}
  if [ -f "$file" ]; then
    while IFS= read -r line || [ -n "$line" ]; do
      case $line in '' | '#'*) continue ;; esac
      case $line in *=*) ;; *)
        echo "Invalid repository setting: $line" >&2
        return 1
        ;;
      esac
      key=$(_trim "${line%%=*}")
      case $_allowed in *" $key "*) ;; *)
        echo "Invalid repository setting: $key" >&2
        return 1
        ;;
      esac
      mode_value=$(_trim "${line#*=}")
      if [ "$key" = "mode" ] && [ "$mode_value" != "$_pinned_mode" ]; then
        echo "Invalid repository setting: mode" >&2
        return 1
      fi
      if [ "$key" = "$1" ] && [ -z "$found" ]; then
        value=$(_trim "${line#*=}")
        found=yes
      fi
    done <"$file"
  fi
  for wanted in $_required; do
    if [ ! -f "$file" ] || ! grep -Eq "^[[:space:]]*${wanted}[[:space:]]*=" "$file"; then
      echo "Missing required repository setting: $wanted" >&2
      return 1
    fi
  done
  printf '%s\n' "$value"
}
