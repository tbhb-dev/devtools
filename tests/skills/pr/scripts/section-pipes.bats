#!/usr/bin/env bats
setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  pr_setup
}

@test "section checks handle lists beyond the pipe buffer (#591)" {
  # Early matches must not close a pipe while printf still has data.
  python3 - <<'PYTHON'
from pathlib import Path
path = Path('PR_AGENTDESC.md')
extra = ('\n## Notes ' + 'x' * 1024 + '\n\nNotes.\n') * 96
path.write_text(path.read_text() + extra)
template = Path('.github/pull_request_template.md')
template.write_text(template.read_text() + extra)
PYTHON
  run /bin/bash "$SCRIPTS/validate-description.sh"
  [ "$status" -eq 0 ]

  sed '/^## Summary$/d' PR_AGENTDESC.md >"$BATS_TEST_TMPDIR/missing.md"
  run /bin/bash "$SCRIPTS/validate-description.sh" "$BATS_TEST_TMPDIR/missing.md"
  [ "$status" -eq 1 ]
  [[ "$output" == *'section-missing  the draft has no "## Summary" section'* ]]

  sed '/^Adds the widget file/d' PR_AGENTDESC.md >"$BATS_TEST_TMPDIR/empty.md"
  run /bin/bash "$SCRIPTS/validate-description.sh" "$BATS_TEST_TMPDIR/empty.md"
  [ "$status" -eq 1 ]
  [[ "$output" == *'section-empty  "## Summary" has no content'* ]]

  sed 's/^## Summary$/## Swapped/;s/^## Why$/## Summary/;s/^## Swapped$/## Why/' PR_AGENTDESC.md >"$BATS_TEST_TMPDIR/reordered.md"
  run /bin/bash "$SCRIPTS/validate-description.sh" "$BATS_TEST_TMPDIR/reordered.md"
  [ "$status" -eq 1 ]
  [[ "$output" == *'section-order  '* ]]
}
