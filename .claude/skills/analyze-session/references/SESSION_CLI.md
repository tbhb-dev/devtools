# `session.py`

The query tool this skill uses. It reads a transcript line by line and prints totals with line numbers, so nothing needs the whole file in context.

It has a uv script shebang and declares which Python version it needs, so it runs directly:

```text
scripts/session.py <subcommand> <transcript> [options]
```

The path is relative to this skill's directory.

## Transcript formats

The tool reads Claude Code transcripts and Codex session files. Each format has its own parser, chosen from the file's first records rather than from the harness running the skill. Either harness can analyze either kind of transcript. Every subcommand takes `--format claude` or `--format codex` to override the detection, and `index` and `meta` print the format they chose. A file matching neither format stops the tool with an error rather than a guess, because the wrong parser reads every record as empty and reports a clean session. `new-retro.sh` and the preflight read `ANALYZE_SESSION_FORMAT` for the same override.

The preflight already runs `index`, `errors`, `loops` and `lint`, and leaves the full output in the retro's `artifacts/`. Run the tool directly when a report needs more detail than the preflight printed, or when the reports point at something to search for.

## `locate`

```text
session.py locate --contains 'some-distinctive-string' [--project SUBSTRING] [--format FORMAT]
```

Finds transcripts by content instead of by a guessed path, searching both stores unless `--format` narrows it. Output is size, path, match count, and format, newest first. `--project` matches the Claude project directory name, or the working directory a Codex session recorded.

Claude Code keeps transcripts under `~/.claude/projects/<project>/`. A session running inside a git worktree writes its transcript under the main repository's project directory, and the worktree's own directory may exist without containing any `.jsonl` at all, so guessing that path finds an empty directory that looks identical to a missing transcript. Codex keeps each session file in a dated directory under `~/.codex/sessions/`. A working directory reveals nothing about that path.

A search with no hits proves nothing on its own. Confirm the same string matches some other transcript before concluding this session is absent.

## `index`

One pass over everything. Prints the format, the record count, how many records are sidechain, the record-type histogram, the tool histogram, and the line numbers of failing results.

The tool histogram shows what kind of session ran. Counting shell calls (`Bash` in a Claude transcript, `exec` in a Codex one) against lint runs can identify a repetition problem on its own.

## `errors`

Every failing result, one per line, classified as `hook-block` (Claude transcripts only), `permission`, `environment`, `lint`, or `nonzero`.

The counts here answer different questions. A Claude result may hold an `is_error` flag, which counts what the harness marked as failed. Codex results have no stable exit code. A shell command exiting nonzero comes back as an ordinary result, as does a linter reporting findings. This subcommand also matches failure text, which makes its count larger than a flag count. Use its count to locate lines for inspection, and say which count a figure came from.

## `loops`

Clusters repeated calls that follow the same pattern, ranked by rounds. `--min-rounds N` sets how few rounds still count, default 3.

A pair of calls counts as one attempt when the signature matches. For a shell call that signature is the tasks and commands invoked rather than the arguments, so repeated edit-then-relint rounds group into one cluster. A Codex `exec` call holds JavaScript orchestration. Its signature uses the nested tool plus the shell command or mise task. Calls more than 40 records apart start a new cluster.

Rounds beyond the first are the wasted round trips. Rank findings by this number.

## `lint`

Linter rule frequency, plus the distribution that matters more than the total. Many failing runs each reporting one or two findings needs a different fix than a few runs reporting many at once. `--top N` lists more rules.

## `grep`

```text
session.py grep <transcript> 'pattern' [--context N] [--kind KIND] [--range LO HI] [--ignore-case] [--first]
```

Searches the decoded content blocks, so what prints matches what the model saw. Shell tools searching the raw file find the record and print it as one escaped line instead.

`--kind` narrows to `TEXT`, `THINK`, `RES`, `EVENT` (Codex only), or a `USE:<tool>` name from the histogram, such as `USE:Bash` or `USE:exec`. Reading `THINK` shows what the agent believed at the time, which is often where an error begins.

## `show`

```text
session.py show <transcript> 361,408,1938  # ranges work too: 40-45
```

Prints whole records, decoded. Use it once a report has given you a specific line number. `--width` limits each block.

## `meta`

Prints `FORMAT`, `SESSION_ID`, `SESSION_DATE`, `RECORDS` and `SIZE_MB` as shell-consumable assignments. `new-retro.sh` uses it so the retro directory takes its date from the session's own last record rather than from whenever the scaffolding runs.
