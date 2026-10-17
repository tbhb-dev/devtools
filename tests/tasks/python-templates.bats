#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/../test_helper.bash"
  isolate_git
  ROOT=$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)
  WORK="${BATS_TEST_TMPDIR}/consumer"
  mkdir -p "$WORK/.devtools/config/python"
  cp "$ROOT/.devtools/config/python/"*.template "$WORK/.devtools/config/python/"
  cd "$WORK" || return
}

@test "root ruff check leaves vendored templates undiscovered" {
  printf '[tool.ruff]\n' > pyproject.toml
  printf '"""Consumer module."""\n' > consumer.py
  run uv run --project "$ROOT" --no-sync ruff check --verbose
  echo "$output"
  [ "$status" -eq 0 ]
  [[ "$output" != *'.devtools/config/python'* ]]
  [ ! -e .devtools/config/python/.ruff_cache ]
  [ ! -e .devtools/config/python/ruff.toml ]
  [ ! -e .devtools/config/python/pyrefly.toml ]
}

@test "adopted templates configure ruff and pyrefly from the consumer root" {
  cp .devtools/config/python/ruff.toml.template ruff.toml
  cp .devtools/config/python/pyrefly.toml.template pyrefly.toml
  mkdir src tests
  printf '"""Consumer module."""\n' > src/consumer.py
  run uv run --project "$ROOT" --no-sync ruff check
  echo "$output"
  [ "$status" -eq 0 ]
  run uv run --project "$ROOT" --no-sync pyrefly check
  echo "$output"
  [ "$status" -eq 0 ]
  printf '\nimport os\n' >> src/consumer.py
  run uv run --project "$ROOT" --no-sync ruff check
  [ "$status" -ne 0 ]
  [[ "$output" == *'F401'* ]]
  printf '\nvalue: int = "wrong"\n' > src/consumer.py
  run uv run --project "$ROOT" --no-sync pyrefly check
  [ "$status" -ne 0 ]
  [[ "$output" == *'bad-assignment'* ]]
}
