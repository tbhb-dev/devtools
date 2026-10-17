#!/usr/bin/env bats

setup() {
  repo_root="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
  cd "$BATS_TEST_TMPDIR"
}

@test "starter words belong to shared tooling and stay sorted and unique" {
  for list in cspell/words.txt vale/accept.txt; do
    words="$repo_root/.devtools/config/$list"
    run sort -cu "$words"
    [ "$status" -eq 0 ]
    while IFS= read -r word; do
      run rg --ignore-case --word-regexp --fixed-strings "$word" \
        "$repo_root/.devtools/tasks" "$repo_root/.config/mise/conf.d"
      [ "$status" -eq 0 ]
    done < "$words"
  done
}

@test "a clean consumer resolves copied cspell and Vale vocabularies" {
  cp "$repo_root/.devtools/config/cspell/words.txt" .cspell-words.txt
  mkdir -p .vale/config/vocabularies/project
  cp "$repo_root/.devtools/config/vale/accept.txt" .vale/config/vocabularies/project/accept.txt
  cat > .cspell.json <<'JSON'
{"version":"0.2","language":"en","dictionaryDefinitions":[{"name":"project","path":"./.cspell-words.txt","addWords":true}],"dictionaries":["project"]}
JSON
  cat > .vale.ini <<'INI'
StylesPath = .vale
Vocab = project
MinAlertLevel = warning
[*.md]
BasedOnStyles = Vale
INI
  cp .cspell-words.txt sample.md
  run cspell lint --no-progress --config .cspell.json sample.md
  [ "$status" -eq 0 ]
  run vale --config .vale.ini sample.md
  [ "$status" -eq 0 ]

  # Removing either copied list must expose the terms it accepts.
  : > .cspell-words.txt
  run cspell lint --no-progress --config .cspell.json sample.md
  [ "$status" -ne 0 ]
  : > .vale/config/vocabularies/project/accept.txt
  run vale --config .vale.ini sample.md
  [ "$status" -eq 1 ]
}
