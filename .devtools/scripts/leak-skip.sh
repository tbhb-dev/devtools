#!/usr/bin/env bash
# Sourced only by the working-tree producer. Match repository-relative paths
# before opening them; object scans must never load this configuration.
load_leak_skip() {
  local line rc
  tree_skip='^$'
  [[ -f .leak-skip ]] || return 0
  while IFS= read -r line || [[ -n $line ]]; do
    [[ $line == \#* || $line != *[![:space:]]* ]] && continue
    [[ $line == ^* ]] || die "skip expressions must start with an anchor"
    rc=0
    printf '' | grep -E "$line" >/dev/null 2>&1 || rc=$?
    ((rc != 2)) || die "invalid skip expression"
    tree_skip="$tree_skip|($line)"
  done <.leak-skip
}
