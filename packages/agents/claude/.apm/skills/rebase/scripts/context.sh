#!/usr/bin/env bash
# Shared discovery bridge. Sourced by the library and standalone readiness.
export LC_ALL=C
unset CDPATH GREP_OPTIONS
IFS=$' \t\n'

# Capability discovery is read-only. Keep the frozen shell path for older
# binaries until #772 retires it; a supported command never falls back.
if [ -z "${_DEVTOOLS_AGENTREPO_CONTEXT+set}" ]; then
  _DEVTOOLS_AGENTREPO_CONTEXT=false
  if [ -n "$_DEVTOOLS_AGENTREPO_BIN" ] &&
    _dt_context_help=$("$_DEVTOOLS_AGENTREPO_BIN" --help 2>/dev/null) &&
    printf '%s\n' "$_dt_context_help" | grep -Eq '^[[:space:]]+context[[:space:]]'; then
    _DEVTOOLS_AGENTREPO_CONTEXT=true
  fi
  unset _dt_context_help
fi

_dt_context() {
  local reply status prefix suffix value failure
  prefix=$'devtools-context-v1\n'
  suffix=$'\ndevtools-context-end'
  failure=$'devtools-context-error-v1\n'
  # The command buffers values and frames failure diagnostics separately.
  # Capture stdout so even a broken producer cannot leak a value.
  if reply=$("$_DEVTOOLS_AGENTREPO_BIN" context --framed --lib-dir "$DT_LIB_DIR" -- "$@"); then
    case $reply in
    "$prefix"*"$suffix") ;;
    *)
      printf 'agentrepo context: incomplete response\n' >&2
      return 1
      ;;
    esac
    value=${reply#"$prefix"}
    value=${value%"$suffix"}
    case $1 in
    agent-context | ready) [ -z "$value" ] || {
      printf 'agentrepo context: unexpected value\n' >&2
      return 1
    } ;;
    *) printf '%s\n' "$value" ;;
    esac
  else
    status=$?
    case $reply in
    "$failure"*"$suffix")
      value=${reply#"$failure"}
      value=${value%"$suffix"}
      printf '%s' "$value" >&2
      ;;
    *) printf 'agentrepo context failed (status %s)\n' "$status" >&2 ;;
    esac
    return "$status"
  fi
}
