# Local event journal

`metrics event` records local work events without network access or a database driver. The devtools Go module includes the command at each release tag. Install it through mise's Go backend, replacing `<release-tag>` with a tag containing this command:

```toml
[tools]
"go:github.com/tbhb-dev/devtools/cmd/metrics" = "<release-tag>"
```

Record an event:

```bash
metrics event --name afakt.agent.run --attr repo=devtools --attr outcome=done
metrics event --name afakt.agent.run --item-ref item-504 --attr outcome=done
```

Names are free-form dotted strings. `--name` must be non-empty. Repeat `--attr key=value` for string attributes. For repeated keys, the last value replaces earlier values. Keys must be non-empty. Values can contain equals signs, commas, and newlines. JSON escaping keeps each event on one line. `--item-ref` is optional.

By default, records go to `$XDG_STATE_HOME/afakt/journal/<UTC date>.jsonl`. When `XDG_STATE_HOME` is empty, the state directory is `$HOME/.local/state`. To select another journal directory, pass an explicit configuration file in Tom's Obvious Minimal Language (TOML):

```toml
journal_dir = "/tmp/event-journal"
```

```bash
metrics event --config metrics.toml --name afakt.agent.run --attr outcome=done
```

An absent or empty `journal_dir` uses the default. Relative directory paths resolve from the working directory, and the command doesn't load a configuration file implicitly. Store endpoints and credentials are outside this command's scope.

Each compact JSON record has `event_name`, `event_id`, `time`, `host`, and `attrs`. `item_ref` appears only when supplied. `time` uses UTC and Request for Comments (RFC) 3339 with nine fractional digits. `host` comes from the local operating system. `event_id` is the hexadecimal SHA-256 digest of host, event name, item reference, timestamp, and decimal process ID, separated by null bytes. Empty attributes produce an empty JSON object.

The encoded line, including its newline, must be at most 4096 bytes. Oversize events fail before opening or changing the journal. A successful command exits 0 without output. Failures print a reason to stderr and exit non-zero. Each record uses one `O_APPEND` write so concurrent processes on a local filesystem keep their lines intact. New directories use mode 0700 and new journal files use mode 0600. Journals are local files, and the append guarantee doesn't cover network filesystems.

Loading journals into a store, reporting, citation checks, and consumer wiring are separate work.
