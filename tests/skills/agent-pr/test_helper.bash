#!/usr/bin/env bash
agent_pr_setup() {
  source "${BATS_TEST_DIRNAME}/../pr/test_helper.bash"
  pr_setup
  SCRIPTS="$ROOT/packages/agents/codex/.apm/skills/agent-pr/scripts"
  export DEVTOOLS_GH_LOGIN=agent
  mkdir .agents
  echo .agents/agent-skills.conf >>.git/info/exclude
}

grant() {
  export CODEX_THREAD_ID=pr-session
  mkdir -p .git/preapprovals
  printf 'pr now unconfirmed\n' >.git/preapprovals/pr-session
}

replace_text() {
  python3 - "$@" <<'PYTHON'
from pathlib import Path
import sys
file = Path(sys.argv[1])
file.write_text(file.read_text().replace(sys.argv[2], sys.argv[3]))
PYTHON
}
