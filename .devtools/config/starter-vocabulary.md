# Starter vocabularies

Copy these lists once when adopting the shared tooling. Your copies become local policy. You can extend them with your project's terms independently of payload updates, while the larger dictionaries at the devtools repository root belong to devtools itself.

```sh
cp .devtools/config/cspell/words.txt .cspell-words.txt
mkdir -p .vale/config/vocabularies/project
cp .devtools/config/vale/accept.txt .vale/config/vocabularies/project/accept.txt
```

For cspell, add a dictionary definition to your root configuration and enable it. Merge these fields with any existing settings.

```json
{
  "language": "en",
  "dictionaryDefinitions": [
    {
      "name": "project",
      "path": "./.cspell-words.txt",
      "addWords": true
    }
  ],
  "dictionaries": ["project"]
}
```

Use the following Vale settings in your root `.vale.ini`, merging them with your existing styles and package settings. The vocabulary path follows `StylesPath` and the vocabulary name in `Vocab`.

```ini
StylesPath = .vale
Vocab = project
MinAlertLevel = warning

[*.md]
BasedOnStyles = Vale
```

## Word audit

Both starter lists contain only the tool names below. Each name appears in the shared payload and refers to a tool consumers use. Keep the lists sorted and free of duplicates.

| Word | Shared payload evidence | Reason |
| --- | --- | --- |
| cspell | `.config/mise/conf.d/10-devtools-tools.toml` | Shared spelling checker |
| mise | `.devtools/tasks/pins.toml` | Task runner and tool manager |
| prek | `.devtools/tasks/vendir.toml` | Hook runner |
| shellcheck | `.config/mise/conf.d/10-devtools-tools.toml` | Shell linter |
| vendir | `.devtools/tasks/vendir.toml` | Payload sync tool |

## Verification

Run `mise exec -- bats tests/config/starter-vocabulary.bats` from the devtools repository. The suite checks every word against the shared tasks and tool configuration. It creates a temporary consumer outside the repository with copies of both lists and local cspell and Vale configurations, then runs the installed tools against text containing the starter words. Next, it empties each copied list. The suite checks that the corresponding tool reports spelling errors, proving that both tools resolve the consumer's own vocabulary paths.
