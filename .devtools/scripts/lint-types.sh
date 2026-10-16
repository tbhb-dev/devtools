#!/usr/bin/env bash
# lint-types — run pyrefly, surviving the agent-worktree blind spot.
#
# pyrefly skips any include pattern sitting behind a hidden path
# component, and an agent worktree lives under .claude/worktrees, so
# inside one every include is skipped and pyrefly fails over an empty
# view: "No Python files matched patterns". That failure says where the
# checkout is, not whether the types hold, and it blocked every commit
# that staged a Python file from a worktree, because the prek pyrefly
# hook runs this task. CI and the main checkout sit on ordinary paths
# where the same invocation sees the full tree, so the gate keeps its
# teeth where the verdict means something.
#
# The pass is deliberately narrow: both conditions must hold, the
# checkout under .claude/worktrees and the empty-view message in the
# output. An empty view anywhere else still fails, so a config edit
# that orphans every include cannot ride through on this exemption.
set -euo pipefail

export LC_ALL=C

status=0
out=$(uv run pyrefly check 2>&1) || status=$?

if [ "$status" -eq 0 ]; then
  printf '%s\n' "$out"
  exit 0
fi

case "$PWD" in
*/.claude/worktrees/*)
  if grep -q 'No Python files matched patterns' <<<"$out"; then
    echo "lint-types: pyrefly matched no files behind the worktree's hidden path; skipping — CI checks the real tree"
    exit 0
  fi
  ;;
esac

printf '%s\n' "$out"
exit "$status"
