# Keep scratch repositories independent of the invoking Git environment.
isolate_git() {
  local name
  for name in ${!GIT_CONFIG_@} ${!GIT_AUTHOR_@} ${!GIT_COMMITTER_@}; do
    unset "$name"
  done
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
}
